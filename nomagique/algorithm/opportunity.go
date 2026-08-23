package algorithm

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolOpportunitySupport          = nomagique.MustIntern("opportunity_support")
	SymbolOpportunityContradiction    = nomagique.MustIntern("opportunity_contradiction")
	SymbolOpportunityConditions       = nomagique.MustIntern("opportunity_conditions")
	SymbolOpportunityConfidenceMass   = nomagique.MustIntern("opportunity_confidence_mass")
	SymbolOpportunityConfidenceWeight = nomagique.MustIntern("opportunity_confidence_weight")
	SymbolOpportunityBalance          = nomagique.MustIntern("opportunity_balance")
	SymbolOpportunityScore            = nomagique.MustIntern("opportunity_score")
	SymbolOpportunityDirection        = nomagique.MustIntern("opportunity_direction")
	SymbolOpportunityConfidence       = nomagique.MustIntern("opportunity_confidence")
	SymbolOpportunityReady            = nomagique.MustIntern("opportunity_ready")

	SymbolEdgeWeight     = nomagique.MustIntern("edge_weight")
	SymbolEdgeConfidence = nomagique.MustIntern("edge_confidence")
	SymbolEdgeRelation   = nomagique.MustIntern("edge_relation")
)

/*
OpportunityReducer processes a single graph edge represented by a types.Frame
and accumulates it into the support, contradiction, or conditions of the opportunity.
*/
func OpportunityReducer(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	weight, foundWeight := input.Get(SymbolEdgeWeight)
	confidence, foundConf := input.Get(SymbolEdgeConfidence)
	relation, foundRel := input.Get(SymbolEdgeRelation)

	if !foundWeight || !foundConf || !foundRel || weight <= 0 || confidence <= 0 {
		return state, state, nil
	}

	nextState := state
	support, _ := state.Get(SymbolOpportunitySupport)
	contradiction, _ := state.Get(SymbolOpportunityContradiction)
	conditions, _ := state.Get(SymbolOpportunityConditions)
	confMass, _ := state.Get(SymbolOpportunityConfidenceMass)
	confWeight, _ := state.Get(SymbolOpportunityConfidenceWeight)

	switch relation {
	case 1:
		nextState.Put(SymbolOpportunitySupport, support+weight)
		nextState.Put(SymbolOpportunityConfidenceMass, confMass+(weight*confidence))
		nextState.Put(SymbolOpportunityConfidenceWeight, confWeight+weight)
	case -1:
		nextState.Put(SymbolOpportunityContradiction, contradiction+weight)
		nextState.Put(SymbolOpportunityConfidenceMass, confMass+(weight*confidence))
		nextState.Put(SymbolOpportunityConfidenceWeight, confWeight+weight)
	default:
		nextState.Put(SymbolOpportunityConditions, conditions+weight)
	}

	return nextState, nextState, nil
}

/*
OpportunityScorer extracts the calculated totals from the OpportunityReducer
and computes the final dimensionless Score, Balance, and Confidence for the graph proposition.
*/
func OpportunityScorer(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	nextState := state
	support, _ := state.Get(SymbolOpportunitySupport)
	contradiction, _ := state.Get(SymbolOpportunityContradiction)
	confMass, _ := state.Get(SymbolOpportunityConfidenceMass)
	confWeight, _ := state.Get(SymbolOpportunityConfidenceWeight)

	directional := support + contradiction

	if directional > 0 && confWeight > 0 {
		balance := (support - contradiction) / directional
		conf := confMass / confWeight

		evidence := directional
		if evidence > 1 {
			evidence = 1
		}

		score := balance * evidence

		nextState.Put(SymbolOpportunityBalance, balance)
		nextState.Put(SymbolOpportunityConfidence, conf)
		nextState.Put(SymbolOpportunityScore, score)
		nextState.Put(SymbolOpportunityReady, 1)

		if score > 0 {
			nextState.Put(SymbolOpportunityDirection, 1)
		} else if score < 0 {
			nextState.Put(SymbolOpportunityDirection, -1)
		}
	} else {
		nextState.Put(SymbolOpportunityReady, 0)
	}

	return nextState, nextState, nil
}

/*
OpportunityPrimitive is the composite algebra that evaluates an opportunity graph.
*/
var OpportunityPrimitive = nomagique.Pipe(
	OpportunityReducer,
	OpportunityScorer,
)
