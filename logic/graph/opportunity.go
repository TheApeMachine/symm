package graph

import (
	"strings"
	"time"

	"github.com/theapemachine/symm/logic/opportunity"
	"github.com/theapemachine/symm/types"
)

/*
OpportunityScore is one archetype's conjunctive evidence score for the current
graph: the trust-attenuated share of supporting legs that hold minus the share
of contradicting legs that hold. A leg holds when a graph node carries that
source/metric identity with a strength above the leg's epistemic floors.
*/
type OpportunityScore struct {
	Type       types.OpportunityType
	Support    float64
	Opposition float64
	Score      float64
	Lifecycle  types.OpportunityLifecycle
}

/*
classifyOpportunities scores every catalog archetype against the graph's nodes.
It never invents an observation: a leg with no matching node contributes zero,
and every leg's vote is scaled by the node's epistemic trust, so a fresh,
ambiguous, or stale reading can support nothing.
*/
func (graph *Graph) classifyOpportunities(now time.Time) []OpportunityScore {
	if graph == nil {
		return nil
	}

	views := graph.reasoningNodeViews()
	scores := make([]OpportunityScore, 0, len(opportunity.Catalog))

	for _, archetype := range opportunity.Catalog {
		score := OpportunityScore{
			Type:      archetype.Type,
			Lifecycle: types.LifecycleEmergent,
		}

		var supportSum float64
		var supportMax float64
		var opposeSum float64
		var opposeMax float64

		for _, leg := range archetype.Supports {
			trust := graph.legTrust(views, leg, now)
			supportSum += trust
			supportMax++
		}

		for _, leg := range archetype.Opposes {
			trust := graph.legTrust(views, leg, now)
			opposeSum += trust
			opposeMax++
		}

		if supportMax > 0 {
			score.Support = supportSum / supportMax
		}

		if opposeMax > 0 {
			score.Opposition = opposeSum / opposeMax
		}

		score.Score = score.Support - score.Opposition

		if score.Score > 0 {
			score.Lifecycle = opportunityLifecycle(score.Support, score.Opposition)
		}

		scores = append(scores, score)
	}

	return scores
}

/*
ActiveOpportunity returns the archetype with the strongest positive evidence for
the current graph, or None when no archetype carries net support.
*/
func (graph *Graph) ActiveOpportunity(now time.Time) OpportunityScore {
	best := OpportunityScore{Type: types.OpportunityNone, Lifecycle: types.LifecycleEmergent}

	for _, score := range graph.classifyOpportunities(now) {
		if score.Score > best.Score {
			best = score
		}
	}

	return best
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

		if node == nil || node.Confidence <= 0 {
			continue
		}

		total += computeObservationTrust(node, now)
		count++
	}

	if count == 0 {
		return 0
	}

	return total / float64(count)
}

func (graph *Graph) legTrust(
	views []reasoningNodeView,
	leg types.ObservationCondition,
	now time.Time,
) float64 {
	for _, view := range views {
		if !legMatches(view, leg) {
			continue
		}

		if leg.MaturityFloor > 0 && view.Confidence <= 0 {
			return 0
		}

		if view.Value <= 0 {
			return 0
		}

		node := graph.Nodes[view.ID]

		if node == nil {
			continue
		}

		trust := computeObservationTrust(node, now)

		if trust <= 0 {
			return 0
		}

		return trust
	}

	return 0
}

func legMatches(view reasoningNodeView, leg types.ObservationCondition) bool {
	if leg.Source != "" && !strings.EqualFold(view.Source, string(leg.Source)) {
		return false
	}

	if leg.Metric != "" && !strings.EqualFold(view.Metric, leg.Metric) {
		return false
	}

	return true
}

func opportunityLifecycle(support float64, opposition float64) types.OpportunityLifecycle {
	switch {
	case opposition > support:
		return types.LifecycleExhausting
	case support > 0.8:
		return types.LifecycleClimax
	case support > 0.55:
		return types.LifecycleAccelerating
	case support > 0.3:
		return types.LifecycleConfirming
	default:
		return types.LifecycleEmergent
	}
}
