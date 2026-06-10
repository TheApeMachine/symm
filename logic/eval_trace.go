package logic

import (
	"fmt"
	"strconv"
	"strings"
)

const firstEntryBranchIndex = 5

/*
TraceNode is one playbook gate visited during an evaluation.
*/
type TraceNode struct {
	Key        string
	Label      string
	Held       bool
	Conditions []TraceCondition
}

/*
TraceCondition records whether a single leaf held at a gate.
*/
type TraceCondition struct {
	Label   string
	Negated bool
	Held    bool
}

/*
EvalTrace collects the path an evaluation took through the playbook tree.
*/
type EvalTrace struct {
	Nodes []TraceNode
}

/*
RecordNode appends a gate result after its condition group is evaluated.
*/
func (trace *EvalTrace) RecordNode(
	key string,
	group *ConditionGroup,
	held bool,
	measurements []Measurement,
	evalContext *EvalContext,
) {
	if trace == nil || group == nil || key == "" {
		return
	}

	node := TraceNode{
		Key:   key,
		Label: conditionGroupLabel(group),
		Held:  held,
	}

	for _, condition := range group.Conditions {
		matched, err := condition.Evaluate(measurements, evalContext)

		if err != nil {
			continue
		}

		node.Conditions = append(node.Conditions, TraceCondition{
			Label:   conditionLabel(condition),
			Negated: condition.Type == ConditionIsFalse,
			Held:    matched,
		})
	}

	trace.Nodes = append(trace.Nodes, node)
}

/*
Bottleneck returns the deepest gate that blocked the evaluation.
*/
func (trace *EvalTrace) Bottleneck() *TraceNode {
	if trace == nil || len(trace.Nodes) == 0 {
		return nil
	}

	for index := len(trace.Nodes) - 1; index >= 0; index-- {
		node := trace.Nodes[index]

		if !node.Held {
			return &trace.Nodes[index]
		}
	}

	return &trace.Nodes[len(trace.Nodes)-1]
}

/*
FailedConditionLabels lists condition labels that did not hold at the bottleneck.
*/
func (trace *EvalTrace) FailedConditionLabels() []string {
	bottleneck := trace.Bottleneck()

	if bottleneck == nil {
		return nil
	}

	labels := make([]string, 0)

	for _, condition := range bottleneck.Conditions {
		if condition.Held {
			continue
		}

		label := condition.Label

		if condition.Negated {
			label = "¬ " + label
		}

		labels = append(labels, label)
	}

	return labels
}

/*
SnapshotSignals compresses the measurement window to the latest category per source.
*/
func SnapshotSignals(measurements []Measurement) map[string]string {
	snapshot := make(map[string]string)

	for _, measurement := range measurements {
		if measurement.Source == "" || measurement.Category == CategoryTypeNone {
			continue
		}

		source := string(measurement.Source)

		snapshot[source] = fmt.Sprintf(
			"%s@%.2f/%.2f",
			measurement.Category,
			measurement.Confidence,
			measurement.Surprise,
		)
	}

	return snapshot
}

/*
Depth returns how many gates deep the trace traveled.
*/
func (trace *EvalTrace) Depth() int {
	if trace == nil {
		return 0
	}

	return len(trace.Nodes)
}

/*
PathKey returns the bottleneck gate key for dedupe keys.
*/
func (trace *EvalTrace) PathKey() string {
	bottleneck := trace.Bottleneck()

	if bottleneck == nil {
		return ""
	}

	return bottleneck.Key
}

/*
Adopt replaces this trace with the nodes from another trace.
*/
func (trace *EvalTrace) Adopt(other *EvalTrace) {
	if trace == nil || other == nil {
		return
	}

	trace.Nodes = append([]TraceNode(nil), other.Nodes...)
}

/*
AuditScore ranks how far an entry-path evaluation progressed before blocking.
Higher depth wins; ties prefer earlier entry branches (ignition first).
*/
func (trace *EvalTrace) AuditScore() (depth int, branchIndex int) {
	if trace == nil || len(trace.Nodes) == 0 || !trace.Nodes[0].Held {
		return 0, 999
	}

	rootKey := strings.Split(trace.Nodes[0].Key, "/")[0]
	branchIndex, err := strconv.Atoi(rootKey)

	if err != nil {
		return 0, 999
	}

	bottleneck := trace.Bottleneck()

	if bottleneck == nil {
		return 0, branchIndex
	}

	depth = strings.Count(bottleneck.Key, "/") + 1

	if !bottleneck.Held {
		return depth, branchIndex
	}

	return len(trace.Nodes), branchIndex
}

/*
BeatsAuditScore reports whether this trace is a better audit candidate.
*/
func (trace *EvalTrace) BeatsAuditScore(depth int, branchIndex int) bool {
	candidateDepth, candidateBranch := trace.AuditScore()

	if candidateDepth != depth {
		return candidateDepth > depth
	}

	return candidateBranch < branchIndex
}
