package logic

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type conditionStat struct {
	label   string
	negated bool
	held    int64
}

type nodeStat struct {
	reached    int64
	held       int64
	conditions []conditionStat
}

/*
TreeStats accumulates playbook reach/hold counts between UI publishes.
*/
type TreeStats struct {
	mu          sync.Mutex
	evaluations int64
	nodes       map[string]*nodeStat
	staticNodes []map[string]any
	recent      []map[string]any
	maxRecent   int
}

func NewTreeStats(branches []*Branch, maxRecent int) *TreeStats {
	stats := &TreeStats{
		nodes:       make(map[string]*nodeStat),
		staticNodes: flattenBranches(branches, "", "", 0),
		maxRecent:   maxRecent,
	}

	return stats
}

func (stats *TreeStats) BeginEvaluation() {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	stats.evaluations++
}

func (stats *TreeStats) Reach(key string) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	node := stats.node(key)
	node.reached++
}

func (stats *TreeStats) Hold(key string) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	node := stats.node(key)
	node.held++
}

func (stats *TreeStats) RecordConditions(
	key string,
	group *ConditionGroup,
	measurements []Measurement,
	evalContext *EvalContext,
) {
	if group == nil {
		return
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	node := stats.node(key)

	for index, condition := range group.Conditions {
		if index >= len(node.conditions) {
			continue
		}

		matched, err := condition.Evaluate(measurements, evalContext)

		if err != nil || !matched {
			continue
		}

		node.conditions[index].held++
	}
}

func (stats *TreeStats) RecordAction(symbol string, evaluation *Evaluation, verdict string, reason string) {
	if evaluation == nil || evaluation.Action == nil || stats.maxRecent <= 0 {
		return
	}

	action := evaluation.Action

	stats.mu.Lock()
	defer stats.mu.Unlock()

	stats.recent = append([]map[string]any{{
		"symbol":  symbol,
		"action":  action.Type.String(),
		"side":    string(action.Side),
		"key":     evaluation.Key,
		"verdict": verdict,
		"reason":  reason,
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
	}}, stats.recent...)

	if len(stats.recent) > stats.maxRecent {
		stats.recent = stats.recent[:stats.maxRecent]
	}
}

/*
DecisionTreeFrame flushes interval stats and returns the dashboard payload.
*/
func (stats *TreeStats) DecisionTreeFrame() map[string]any {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	nodes := make([]map[string]any, 0, len(stats.staticNodes))

	for _, static := range stats.staticNodes {
		key, _ := static["key"].(string)
		node := stats.nodes[key]

		reached := int64(0)
		held := int64(0)
		conditions := static["conditions"].([]map[string]any)

		if node != nil {
			reached = node.reached
			held = node.held

			merged := make([]map[string]any, len(conditions))

			for index, condition := range conditions {
				merged[index] = map[string]any{
					"label":   condition["label"],
					"negated": condition["negated"],
					"held":    int64(0),
				}

				if index < len(node.conditions) {
					merged[index]["held"] = node.conditions[index].held
				}
			}

			conditions = merged
		}

		frame := map[string]any{
			"key":        static["key"],
			"depth":      static["depth"],
			"parent":     static["parent"],
			"label":      static["label"],
			"action":     static["action"],
			"reached":    reached,
			"held":       held,
			"combinator": static["combinator"],
			"conditions": conditions,
		}

		nodes = append(nodes, frame)
	}

	frame := map[string]any{
		"chart":       "decision_tree",
		"evaluations": stats.evaluations,
		"nodes":       nodes,
		"recent":      append([]map[string]any(nil), stats.recent...),
	}

	stats.evaluations = 0
	stats.nodes = make(map[string]*nodeStat)

	return frame
}

func (stats *TreeStats) node(key string) *nodeStat {
	node, ok := stats.nodes[key]

	if ok {
		return node
	}

	static := stats.staticByKey(key)
	conditions := make([]conditionStat, 0)

	if static != nil {
		rawConditions, _ := static["conditions"].([]map[string]any)

		for _, condition := range rawConditions {
			label, _ := condition["label"].(string)
			negated, _ := condition["negated"].(bool)

			conditions = append(conditions, conditionStat{
				label:   label,
				negated: negated,
			})
		}
	}

	node = &nodeStat{conditions: conditions}
	stats.nodes[key] = node

	return node
}

func (stats *TreeStats) staticByKey(key string) map[string]any {
	for _, node := range stats.staticNodes {
		if node["key"] == key {
			return node
		}
	}

	return nil
}

func flattenBranches(
	branches []*Branch,
	parent string,
	prefix string,
	depth int,
) []map[string]any {
	nodes := make([]map[string]any, 0)

	for index, branch := range branches {
		key := prefix + strconv.Itoa(index)
		nodes = append(nodes, branch.staticNode(key, parent, depth))
		nodes = append(
			nodes,
			flattenBranches(branch.Branches, key, key+"/", depth+1)...,
		)
	}

	return nodes
}

func (branch *Branch) staticNode(key, parent string, depth int) map[string]any {
	label := "branch"
	combinator := "single"
	conditions := make([]map[string]any, 0)

	if branch.ConditionGroup != nil {
		label = conditionGroupLabel(branch.ConditionGroup)
		combinator = booleanLabel(branch.ConditionGroup.Boolean)

		for _, condition := range branch.ConditionGroup.Conditions {
			conditions = append(conditions, map[string]any{
				"label":   conditionLabel(condition),
				"negated": condition.Type == ConditionIsFalse,
				"held":    int64(0),
			})
		}
	}

	action := ""

	if branch.Action != nil {
		action = branch.Action.Type.String()
	}

	return map[string]any{
		"key":        key,
		"depth":      depth,
		"parent":     parent,
		"label":      label,
		"action":     action,
		"combinator": combinator,
		"conditions": conditions,
	}
}

func booleanLabel(boolean BooleanType) string {
	switch boolean {
	case BooleanTypeAnd:
		return "all"
	case BooleanTypeOr:
		return "any"
	default:
		return "single"
	}
}

func conditionGroupLabel(group *ConditionGroup) string {
	labels := make([]string, 0, len(group.Conditions))

	for _, condition := range group.Conditions {
		label := conditionLabel(condition)

		if condition.Type == ConditionIsFalse {
			label = "¬ " + label
		}

		labels = append(labels, label)
	}

	separator := " · "

	if group.Boolean == BooleanTypeOr {
		separator = " | "
	}

	return strings.Join(labels, separator)
}

func conditionLabel(condition Condition) string {
	switch condition.Type {
	case ConditionIsTrue, ConditionIsFalse:
		return subjectLabel(condition.Left.Subject)
	case ConditionIsEqual, ConditionIsNotEqual:
		return subjectLabel(condition.Left.Subject) + " = " + subjectLabel(condition.Right.Subject)
	case ConditionIsGreaterThan:
		return subjectLabel(condition.Left.Subject) + " > " + scalarLabel(condition.Right.Subject)
	case ConditionIsLessThan:
		return subjectLabel(condition.Left.Subject) + " < " + scalarLabel(condition.Right.Subject)
	case ConditionIsGreaterThanOrEqual:
		return subjectLabel(condition.Left.Subject) + " ≥ " + scalarLabel(condition.Right.Subject)
	case ConditionIsLessThanOrEqual:
		return subjectLabel(condition.Left.Subject) + " ≤ " + scalarLabel(condition.Right.Subject)
	case ConditionIsWithin:
		return subjectLabel(condition.Left.Subject) + " ~ " + scalarLabel(condition.Right.Subject)
	case ConditionIsNotWithin:
		return subjectLabel(condition.Left.Subject) + " ≁ " + scalarLabel(condition.Right.Subject)
	default:
		return "condition"
	}
}

func subjectLabel(subject Subject) string {
	source := string(subject.Source)

	switch subject.Type {
	case SubjectCategory:
		if subject.Category == nil {
			return source + ".category"
		}

		return source + "." + string(subject.Category.Type)
	case SubjectRegime:
		if subject.Regime == nil {
			return source + ".regime"
		}

		return source + "." + string(subject.Regime.Type)
	case SubjectPosition:
		if subject.Position == nil {
			return source + ".position"
		}

		return source + "." + string(subject.Position.Type)
	case SubjectHolding:
		if subject.Holding == nil {
			return "holding"
		}

		if subject.Holding.Held {
			return "holding"
		}

		return "not_holding"
	case SubjectConfidence:
		return source + ".confidence"
	case SubjectSurprise:
		return source + ".surprise"
	case SubjectStrength:
		return source + ".strength"
	default:
		return fmt.Sprintf("%s.%d", source, subject.Type)
	}
}

func scalarLabel(subject Subject) string {
	if value, ok := subject.threshold(); ok {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}

	return subjectLabel(subject)
}
