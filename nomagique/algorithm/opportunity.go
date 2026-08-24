package algorithm

import (
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolOpportunitySupport          = types.MustIntern("opportunity_support")
	SymbolOpportunityContradiction    = types.MustIntern("opportunity_contradiction")
	SymbolOpportunityConditions       = types.MustIntern("opportunity_conditions")
	SymbolOpportunityConfidenceMass   = types.MustIntern("opportunity_confidence_mass")
	SymbolOpportunityConfidenceWeight = types.MustIntern("opportunity_confidence_weight")
	SymbolOpportunityBalance          = types.MustIntern("opportunity_balance")
	SymbolOpportunityScore            = types.MustIntern("opportunity_score")
	SymbolOpportunityDirection        = types.MustIntern("opportunity_direction")
	SymbolOpportunityConfidence       = types.MustIntern("opportunity_confidence")
	SymbolOpportunityReady            = types.MustIntern("opportunity_ready")

	SymbolEdgeWeight     = types.MustIntern("edge_weight")
	SymbolEdgeConfidence = types.MustIntern("edge_confidence")
	SymbolEdgeRelation   = types.MustIntern("edge_relation")
)

/*
OpportunityReducer processes a single graph edge represented by a types.Frame
and accumulates it into the support, contradiction, or conditions of the opportunity.
*/
func OpportunityReducer(input types.Frame) types.Frame {
	weight, foundWeight := input.Get(SymbolEdgeWeight)
	confidence, foundConf := input.Get(SymbolEdgeConfidence)
	relation, foundRel := input.Get(SymbolEdgeRelation)

	if !foundWeight || !foundConf || !foundRel || weight <= 0 || confidence <= 0 {
		return input
	}

	support, _ := input.Get(SymbolOpportunitySupport)
	contradiction, _ := input.Get(SymbolOpportunityContradiction)
	conditions, _ := input.Get(SymbolOpportunityConditions)
	confMass, _ := input.Get(SymbolOpportunityConfidenceMass)
	confWeight, _ := input.Get(SymbolOpportunityConfidenceWeight)

	switch relation {
	case 1:
		input.Put(SymbolOpportunitySupport, support+weight)
		input.Put(SymbolOpportunityConfidenceMass, confMass+(weight*confidence))
		input.Put(SymbolOpportunityConfidenceWeight, confWeight+weight)
	case -1:
		input.Put(SymbolOpportunityContradiction, contradiction+weight)
		input.Put(SymbolOpportunityConfidenceMass, confMass+(weight*confidence))
		input.Put(SymbolOpportunityConfidenceWeight, confWeight+weight)
	default:
		input.Put(SymbolOpportunityConditions, conditions+weight)
	}

	return input
}

/*
OpportunityScorer extracts the calculated totals from the OpportunityReducer
and computes the final dimensionless Score, Balance, and Confidence for the graph proposition.
*/
func OpportunityScorer(input types.Frame) types.Frame {
	support, _ := input.Get(SymbolOpportunitySupport)
	contradiction, _ := input.Get(SymbolOpportunityContradiction)
	confMass, _ := input.Get(SymbolOpportunityConfidenceMass)
	confWeight, _ := input.Get(SymbolOpportunityConfidenceWeight)

	directional := support + contradiction

	if directional > 0 && confWeight > 0 {
		balance := (support - contradiction) / directional
		conf := confMass / confWeight

		evidence := directional
		if evidence > 1 {
			evidence = 1
		}

		score := balance * evidence

		input.Put(SymbolOpportunityBalance, balance)
		input.Put(SymbolOpportunityConfidence, conf)
		input.Put(SymbolOpportunityScore, score)
		input.Put(SymbolOpportunityReady, 1)

		if score > 0 {
			input.Put(SymbolOpportunityDirection, 1)
		} else if score < 0 {
			input.Put(SymbolOpportunityDirection, -1)
		}
	} else {
		input.Put(SymbolOpportunityReady, 0)
	}

	return input
}

/*
OpportunityPrimitive is the composite algebra that evaluates an opportunity graph.
*/
var OpportunityPrimitive = types.Pipe(
	OpportunityReducer,
	OpportunityScorer,
)
