package category

import "github.com/theapemachine/symm/types"

/*
Reporter wraps a resident Graph for read-only trap, exhaustion, and DMT reporting.
*/
type Reporter struct {
	graph *Graph
}

/*
Report returns read-only reporting helpers for graph.
*/
func Report(graph *Graph) *Reporter {
	if graph == nil {
		return nil
	}

	return &Reporter{graph: graph}
}

/*
TrapPressure is trap vs opportunity category strength on the resident nodes for
symbol, plus Contradicts edge mass that affinity placed from trap→opportunity.
*/
func (reporter *Reporter) TrapPressure(symbol string) (share float64, dominates bool) {
	if reporter == nil || reporter.graph == nil || symbol == "" {
		return 0, false
	}

	graph := reporter.graph
	var trapMass, opportunityMass, contradict, support float64

	for key, node := range graph.nodes {
		if key.symbol != symbol || node == nil || node.Strength <= 0 {
			continue
		}

		switch {
		case trapCategory(node.Type):
			trapMass += node.Strength
		case opportunityCategory(node.Type):
			opportunityMass += node.Strength
		}
	}

	for _, key := range graph.edgesBySymbol[symbol] {
		relation := graph.edges[key]

		if relation == nil || relation.Weight <= 0 {
			continue
		}

		switch relation.Type {
		case Contradicts:
			if trapCategory(relation.From) && opportunityCategory(relation.To) {
				contradict += relation.Weight
			}
		case Supports:
			if opportunityCategory(relation.From) && opportunityCategory(relation.To) {
				support += relation.Weight
			}
		}
	}

	weightedTrap := trapMass + contradict
	weightedOpportunity := opportunityMass + support
	total := weightedTrap + weightedOpportunity

	if total > 0 {
		share = weightedTrap / total
	}

	dominates = weightedTrap > weightedOpportunity && weightedTrap > 0

	return share, dominates
}

/*
ExhaustionLead is the share of Leads edge weight pointing into exhaustion-family
categories versus opportunity-family categories for one symbol.
*/
func (reporter *Reporter) ExhaustionLead(symbol string) (share float64, dominates bool) {
	if reporter == nil || reporter.graph == nil || symbol == "" {
		return 0, false
	}

	graph := reporter.graph
	var intoExhaustion, intoOpportunity float64

	for _, key := range graph.edgesBySymbol[symbol] {
		relation := graph.edges[key]

		if relation == nil || relation.Type != Leads || relation.Weight <= 0 {
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
Tokens builds the DMT category bag for symbol from the latest composed rows,
including a transition token when the resident prior differs from the current
top.
*/
func (reporter *Reporter) Tokens(
	symbol string, categories []types.Category,
) []string {
	if reporter == nil || reporter.graph == nil {
		return nil
	}

	graph := reporter.graph
	tokens := make([]string, 0, len(categories)+1)

	for _, category := range categories {
		if category.Symbol != symbol || category.Strength <= 0 {
			continue
		}

		tokens = append(tokens, "cat-"+string(category.Type)+"-"+polarity(category.Strength))
	}

	top := Top(categories, symbol)
	prior := graph.Prior(symbol)

	if prior != types.CategoryTypeNone && top.Type != types.CategoryTypeNone && prior != top.Type {
		tokens = append(tokens, "transition-"+string(prior)+"-"+string(top.Type))
	}

	return tokens
}

/*
polarity maps positive strength onto the DMT polarity vocabulary.
*/
func polarity(strength float64) string {
	if strength > 0 {
		return "positive"
	}

	return "zero"
}

/*
trapCategory reports taxonomy nodes that refuse or tax a long entry.
*/
func trapCategory(categoryType types.CategoryType) bool {
	switch categoryType {
	case types.SpoofTrap,
		types.ToxicBluff,
		types.HiddenAbsorption,
		types.VolumeStarvation,
		types.Exhaustion,
		types.FadedExhaustion,
		types.ThermalExhaustion,
		types.MechanicalCollapse,
		types.ActiveReversal:
		return true
	default:
		return false
	}
}

/*
opportunityCategory reports taxonomy nodes that corroborate a real long.
*/
func opportunityCategory(categoryType types.CategoryType) bool {
	switch categoryType {
	case types.VerticalIgnition,
		types.RiskOnSurge,
		types.OrganicTrend,
		types.AggressiveDrive,
		types.CoiledCompression,
		types.LoadedImbalance,
		types.Organic,
		types.Frenzy:
		return true
	default:
		return false
	}
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
