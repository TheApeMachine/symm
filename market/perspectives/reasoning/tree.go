package reasoning

import (
	"fmt"
	"strconv"
	"strings"
)

/*
TreeNode is one node of the playbook flattened for the live decision-tree view.
Key matches the evaluator's node key (the dotted path used by NodeTrace), Parent
is the key of the enclosing node ("" for a top-level branch), Label is a
human-readable form of the node's When predicate, and Action is its decision (""
when the node only thinks without acting).
*/
type TreeNode struct {
	Key        string      `json:"key"`
	Depth      int         `json:"depth"`
	Parent     string      `json:"parent"`
	Label      string      `json:"label"`
	Action     string      `json:"action"`
	Combinator string      `json:"combinator"` // all | any | not | leaf
	Conditions []Condition `json:"conditions"` // the node's leaf predicates, in evaluation order
}

// Condition is one leaf predicate of a node's When, for the per-condition
// pass/fail breakdown in the decision tree.
type Condition struct {
	Label   string `json:"label"`
	Negated bool   `json:"negated"`
}

// LeafRef is a leaf predicate flattened out of a (possibly compound) predicate,
// with whether it sits under an odd number of NOTs.
type LeafRef struct {
	Predicate Predicate
	Negated   bool
}

/*
FlattenLeaves returns a predicate's leaf comparisons in a fixed pre-order, so the
static tree (labels) and the live trace (pass/fail) enumerate them identically.
*/
func FlattenLeaves(predicate Predicate) []LeafRef {
	var leaves []LeafRef

	var walk func(node Predicate, negated bool)

	walk = func(node Predicate, negated bool) {
		switch {
		case len(node.All) > 0:
			for _, sub := range node.All {
				walk(sub, negated)
			}
		case len(node.Any) > 0:
			for _, sub := range node.Any {
				walk(sub, negated)
			}
		case node.Not != nil:
			walk(*node.Not, !negated)
		default:
			leaves = append(leaves, LeafRef{Predicate: node, Negated: negated})
		}
	}

	walk(predicate, false)

	return leaves
}

func combinatorOf(predicate Predicate) string {
	switch {
	case len(predicate.All) > 0:
		return "all"
	case len(predicate.Any) > 0:
		return "any"
	case predicate.Not != nil:
		return "not"
	default:
		return "leaf"
	}
}

func conditionsOf(predicate Predicate) []Condition {
	leaves := FlattenLeaves(predicate)
	conditions := make([]Condition, 0, len(leaves))

	for index := range leaves {
		conditions = append(conditions, Condition{
			Label:   describeLeaf(leaves[index].Predicate),
			Negated: leaves[index].Negated,
		})
	}

	return conditions
}

/*
BuildTree flattens a playbook into nodes keyed identically to the evaluator, so a
ReasonTrace's per-node outcomes line up with this structure for rendering.
*/
func BuildTree(thoughts []Thought) []TreeNode {
	nodes := make([]TreeNode, 0, len(thoughts))

	var walk func(branch []Thought, depth int, prefix, parent string)

	walk = func(branch []Thought, depth int, prefix, parent string) {
		for index := range branch {
			key := prefix + strconv.Itoa(index)

			action := ""

			if branch[index].Do.Type != ActionNone {
				action = branch[index].Do.Type.String()
			}

			nodes = append(nodes, TreeNode{
				Key:        key,
				Depth:      depth,
				Parent:     parent,
				Label:      DescribePredicate(branch[index].When),
				Action:     action,
				Combinator: combinatorOf(branch[index].When),
				Conditions: conditionsOf(branch[index].When),
			})

			walk(branch[index].Then, depth+1, key+".", key)
		}
	}

	walk(thoughts, 0, "", "")

	return nodes
}

/*
DescribePredicate renders a predicate as a compact human-readable one-liner for
the tree view: compound predicates join their operands, leaves read as
"subject op value".
*/
func DescribePredicate(predicate Predicate) string {
	switch {
	case len(predicate.All) > 0:
		return joinPredicates(predicate.All, " & ")
	case len(predicate.Any) > 0:
		return joinPredicates(predicate.Any, " | ")
	case predicate.Not != nil:
		return "!(" + DescribePredicate(*predicate.Not) + ")"
	}

	return describeLeaf(predicate)
}

func joinPredicates(predicates []Predicate, separator string) string {
	parts := make([]string, len(predicates))

	for index := range predicates {
		parts[index] = DescribePredicate(predicates[index])
	}

	return strings.Join(parts, separator)
}

func describeLeaf(predicate Predicate) string {
	switch predicate.Subject {
	case SubjectRegime:
		return "regime = " + predicate.Regime.String()
	case SubjectPosition:
		return "position " + predicate.Lifecycle.String()
	}

	left := subjectNames[predicate.Subject]

	if predicate.Subject == SubjectSignal && predicate.Category != "" {
		left = "signal " + string(predicate.Category)
	}

	// snr / confidence name the signal-strength axis, so they read as part of the
	// subject ("signal X.snr"); other units (%, time, pips) describe the value, so
	// they render as a suffix on the value instead of the subject — otherwise a
	// "price.percentage rose by 0.25" reads as 0.25 of something, not 0.25%.
	switch predicate.Unit {
	case UnitSNR, UnitConfidence:
		left += "." + unitNames[predicate.Unit]
	}

	right := formatRHS(predicate.Value, predicate.Unit)

	if predicate.Versus != nil {
		right = describeOperand(*predicate.Versus)
	}

	out := strings.TrimSpace(left + " " + comparisonSymbol(predicate.Op) + " " + right)

	if predicate.Ago > 0 {
		out += fmt.Sprintf(" [%d ago]", predicate.Ago)
	}

	return out
}

// formatRHS renders a leaf's target value with the unit that makes it legible:
// percentages and time as a suffix on the number, so "rose by 0.25%" can never be
// misread as 25%.
func formatRHS(value float64, unit UnitType) string {
	formatted := formatFloat(value)

	switch unit {
	case UnitPercentage:
		return formatted + "%"
	case UnitTimeMinutes:
		return formatted + "m"
	case UnitTimeSeconds:
		return formatted + "s"
	case UnitTimeHours:
		return formatted + "h"
	case UnitTimeDays:
		return formatted + "d"
	case UnitPips:
		return formatted + " pips"
	case UnitPoints:
		return formatted + " pts"
	case UnitTicks:
		return formatted + " ticks"
	default:
		return formatted
	}
}

func describeOperand(operand Operand) string {
	name := subjectNames[operand.Subject]

	if operand.Subject == SubjectSignal && operand.Category != "" {
		name = "signal " + string(operand.Category)
	}

	if unit := unitNames[operand.Unit]; unit != "" {
		name += "." + unit
	}

	return name
}

func comparisonSymbol(comparison Comparison) string {
	switch comparison {
	case ComparisonAtLeast:
		return "≥"
	case ComparisonAtMost:
		return "≤"
	case ComparisonAbove:
		return ">"
	case ComparisonBelow:
		return "<"
	case ComparisonEquals:
		return "="
	case ComparisonRoseBy:
		return "rose by"
	case ComparisonFellBy:
		return "fell by"
	case ComparisonCrossedUp:
		return "crossed up"
	case ComparisonCrossedDown:
		return "crossed down"
	default:
		return comparisonNames[comparison]
	}
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
