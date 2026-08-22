package strategy

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/symm/types"
)

type weightedEvidence struct {
	value float64
	mass  float64
}

func boundedSigned(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return max(-1, min(1, value))
}

func normalizedValue(node *types.Node) (float64, bool) {
	if node == nil {
		return 0, false
	}

	if node.Normalized != nil && !math.IsNaN(*node.Normalized) &&
		!math.IsInf(*node.Normalized, 0) {
		return boundedSigned(*node.Normalized), true
	}

	return 0, false
}

/*
liquidityNodeEvidence maps only measurements whose directional meaning is
explicit in their metric identity. Dimensioned raw depth is never compared
across symbols. Z-scores and imbalances use their natural dimensionless domain;
normalized hypothesis scores are consumed as published.
*/
func liquidityNodeEvidence(node *types.Node) (float64, bool) {
	if node == nil {
		return 0, false
	}

	source := strings.ToLower(node.Source)
	metric := strings.ToLower(string(node.Metric))

	switch source {
	case string(types.SourceLiquidity):
		switch metric {
		case "depth_zscore", string(types.MetricLiquidityDepthDeviation):
			return math.Tanh(node.Value), true
		case "depth_stability", string(types.MetricLiquidityPresence):
			if value, ok := normalizedValue(node); ok {
				return value, true
			}

			if node.Value >= 0 && node.Value <= 1 {
				return node.Value, true
			}
		}
	case string(types.SourceDepthFlow):
		value, normalized := normalizedValue(node)

		if !normalized {
			value = math.Tanh(node.Value)
		}

		switch metric {
		case string(types.MetricLoadedScore), "touch_imbalance", "deep_imbalance":
			return value, true
		case string(types.MetricThinScore), string(types.MetricSpoofScore):
			return -math.Abs(value), true
		}
	case string(types.SourceToxicity):
		value, normalized := normalizedValue(node)

		if !normalized {
			value = math.Tanh(node.Value)
		}

		switch metric {
		case string(types.MetricSupportScore):
			return value, true
		case string(types.MetricBluffScore), string(types.MetricVacuumScore):
			return -math.Abs(value), true
		}
	}

	return 0, false
}

/*
graphLiquidity returns a confidence-mass weighted score in [-1,1]. The mass is
the graph's own maturity × separation × freshness × skill result already
attached to each node, so immature, ambiguous or stale readings cannot dominate.
*/
func graphLiquidity(graph *types.Graph) (float64, float64) {
	if graph == nil {
		return 0, 0
	}

	weighted := 0.0
	mass := 0.0

	for _, node := range graph.Nodes {
		value, relevant := liquidityNodeEvidence(node)

		if !relevant || node.Confidence <= 0 || math.IsNaN(node.Confidence) ||
			math.IsInf(node.Confidence, 0) {
			continue
		}

		weight := min(1, node.Confidence)
		weighted += boundedSigned(value) * weight
		mass += weight
	}

	if mass == 0 {
		return 0, 0
	}

	return boundedSigned(weighted / mass), mass
}

type directContribution struct {
	label string
	value float64
}

/*
directEvidenceExplanation names the strongest graph claims that directly
address the decision proposition. It uses the exact signed edge contribution
already consumed by the graph: relation sign × weight × confidence.
*/
func directEvidenceExplanation(graph *types.Graph) string {
	if graph == nil || graph.DecisionTarget == "" {
		return ""
	}

	contributions := make([]directContribution, 0)

	for _, edge := range graph.Edges {
		if edge == nil || edge.To != graph.DecisionTarget || edge.Weight <= 0 ||
			edge.Confidence <= 0 {
			continue
		}

		sign := 0.0

		switch edge.Relation {
		case types.RelationSupports:
			sign = 1
		case types.RelationContradicts:
			sign = -1
		default:
			continue
		}

		node := graph.Nodes[edge.From]
		label := edge.From

		if node != nil {
			identity := strings.Trim(strings.Join([]string{
				node.Source,
				string(node.Metric),
			}, "/"), "/")

			if identity != "" {
				label = identity
			}
		}

		contributions = append(contributions, directContribution{
			label: label,
			value: sign * edge.Weight * edge.Confidence,
		})
	}

	if len(contributions) == 0 {
		return ""
	}

	slices.SortStableFunc(contributions, func(left, right directContribution) int {
		leftAbs := math.Abs(left.value)
		rightAbs := math.Abs(right.value)

		switch {
		case leftAbs > rightAbs:
			return -1
		case leftAbs < rightAbs:
			return 1
		default:
			return strings.Compare(left.label, right.label)
		}
	})

	limit := min(4, len(contributions))
	parts := make([]string, 0, limit)

	for _, contribution := range contributions[:limit] {
		direction := "supports"

		if contribution.value < 0 {
			direction = "contradicts"
		}

		parts = append(parts, fmt.Sprintf(
			"%s %s %.3f",
			contribution.label,
			direction,
			math.Abs(contribution.value),
		))
	}

	return "direct evidence: " + strings.Join(parts, ", ")
}
