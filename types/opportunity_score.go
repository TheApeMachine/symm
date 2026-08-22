package types

import (
	"math"
	"strings"
	"time"

)

/*
OpportunityScore is one archetype's conjunctive evidence score for the current
graph: the trust-attenuated share of supporting legs that hold minus the share
of contradicting legs that hold. A leg holds when a graph node carries that
source/metric identity with a strength above the leg's epistemic floors.
*/
type OpportunityScore struct {
	Type       OpportunityType
	Support    float64
	Opposition float64
	Score      float64
	Lifecycle  OpportunityLifecycle
}

/*
ActiveOpportunity returns the archetype with the strongest positive evidence for
the current graph, or None when no archetype carries net support.
*/
func (graph *Graph) ActiveOpportunity(now time.Time) OpportunityScore {
	best := OpportunityScore{Type: OpportunityNone, Lifecycle: LifecycleEmergent}

	if graph == nil {
		return best
	}

	for _, archetype := range Catalog {
		score := graph.opportunityScore(archetype, now)

		if score.Score > best.Score {
			best = score
		}
	}

	return best
}

func (graph *Graph) opportunityScore(
	archetype OpportunityArchetype,
	now time.Time,
) OpportunityScore {
	score := OpportunityScore{Type: archetype.Type, Lifecycle: LifecycleEmergent}
	supportCount := 0
	oppositionCount := 0

	for _, leg := range archetype.Supports {
		if !leg.Supports {
			continue
		}

		score.Support += graph.legTrust(leg, now)
		supportCount++
	}

	for _, leg := range archetype.Opposes {
		if !leg.Contradicts {
			continue
		}

		score.Opposition += graph.legTrust(leg, now)
		oppositionCount++
	}

	if supportCount > 0 {
		score.Support /= float64(supportCount)
	}

	if oppositionCount > 0 {
		score.Opposition /= float64(oppositionCount)
	}

	score.Score = score.Support - score.Opposition

	return score
}

/*
MeanTrust returns the mean epistemic trust across every reasoning-visible node
with positive confidence: the graph-level confidence that its observations are
current, mature, unambiguous, and skilled.
*/
func (graph *Graph) MeanTrust(now time.Time) float64 {
	if graph == nil {
		return 0
	}

	total := 0.0
	count := 0

	for _, view := range graph.reasoningNodeViews() {
		node := graph.Nodes[view.ID]

		if node == nil {
			continue
		}

		trust := computeObservationTrust(node, now)

		if trust <= 0 {
			continue
		}

		total += trust
		count++
	}

	if count == 0 {
		return 0
	}

	return total / float64(count)
}

func (graph *Graph) legTrust(
	leg ObservationCondition,
	now time.Time,
) float64 {
	bestTrust := 0.0

	for _, node := range graph.Nodes {
		if node == nil || !legMatches(node, leg) {
			continue
		}

		if node.Value <= 0 && (node.Normalized == nil || *node.Normalized <= 0) {
			continue
		}

		trust := computeObservationTrust(node, now) * NodeInfluence(node)

		if trust > bestTrust {
			bestTrust = trust
		}
	}

	return bestTrust
}

func NodeInfluence(node *Node) float64 {
	if node == nil {
		return 0
	}

	if node.Normalized != nil {
		return math.Abs(*node.Normalized)
	}

	return 1
}

func legMatches(
	node *Node,
	leg ObservationCondition,
) bool {
	if leg.Source != "" && !strings.EqualFold(node.Source, string(leg.Source)) {
		return false
	}

	if leg.Metric != "" && !strings.EqualFold(string(node.Metric), leg.Metric) {
		return false
	}

	if leg.Side != SideNone && node.Side != leg.Side {
		return false
	}

	return true
}

func (graph *Graph) observationSeparation(
	node *Node,
) (float64, bool, bool) {
	separation, found := node.Metadata["hypothesis_separation"].(float64)

	if found {
		return separation, true, true
	}

	if node.MeasurementID == "" {
		return 0, false, false
	}

	for _, candidate := range graph.Nodes {
		if candidate == nil || candidate.MeasurementID != node.MeasurementID ||
			!strings.EqualFold(candidate.Source, node.Source) ||
			candidate.Metric != MetricHypothesisSeparation {
			continue
		}

		return candidate.Value, false, true
	}

	return 0, false, false
}
