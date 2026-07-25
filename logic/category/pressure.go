package category

import "github.com/theapemachine/symm/types"

/*
ExhaustionLead is the share of Leads edge weight pointing into exhaustion-family
categories versus opportunity-family categories for one symbol.
*/
func (graph *Graph) ExhaustionLead(symbol string) (share float64, dominates bool) {
	if graph == nil || symbol == "" {
		return 0, false
	}

	var intoExhaustion, intoOpportunity float64

	for key, relation := range graph.edges {
		if key.symbol != symbol || relation == nil || relation.Type != Leads || relation.Weight <= 0 {
			continue
		}

		switch {
		case exhaustionCategory(relation.To):
			intoExhaustion += relation.Weight
		case opportunityCategory(relation.To):
			intoOpportunity += relation.Weight
		}
	}

	total := intoExhaustion + intoOpportunity

	if total <= 0 {
		return 0, false
	}

	share = intoExhaustion / total
	dominates = intoExhaustion > intoOpportunity && intoExhaustion > 0

	return share, dominates
}

/*
exhaustionCategory reports taxonomy nodes that terminate a long continuation.
*/
func exhaustionCategory(categoryType types.CategoryType) bool {
	switch categoryType {
	case types.Exhaustion,
		types.FadedExhaustion,
		types.ThermalExhaustion,
		types.MechanicalCollapse,
		types.ActiveReversal,
		types.LiquidityVacuum,
		types.VolumeStarvation:
		return true
	default:
		return false
	}
}
