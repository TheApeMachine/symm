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
func Report(graph *Graph) Reporter {
	return Reporter{graph: graph}
}

/*
TrapPressure is trap vs opportunity category strength on the resident nodes for
symbol, plus Contradicts edge mass that affinity placed from trap→opportunity.
*/
func (reporter Reporter) TrapPressure(symbol string) (share float64, dominates bool) {
	if reporter.graph == nil || symbol == "" {
		return 0, false
	}

	graph := reporter.graph
	graph.mu.RLock()
	defer graph.mu.RUnlock()

	var trapMass, opportunityMass, contradict, support float64

	for key, node := range graph.NodeIndex {
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
		relation := graph.EdgeIndex[key]

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
func (reporter Reporter) ExhaustionLead(symbol string) (share float64, dominates bool) {
	if reporter.graph == nil || symbol == "" {
		return 0, false
	}

	graph := reporter.graph
	graph.mu.RLock()
	defer graph.mu.RUnlock()

	var intoExhaustion, intoOpportunity float64

	for _, key := range graph.edgesBySymbol[symbol] {
		relation := graph.EdgeIndex[key]

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
func (reporter Reporter) Tokens(
	symbol string, categories []types.Category,
) []string {
	if reporter.graph == nil {
		return nil
	}

	top := Top(categories)
	prior := reporter.graph.Prior(symbol)
	tokens := make([]string, 0, 2)

	if prior != types.CategoryTypeNone {
		tokens = append(tokens, string(prior))
	}

	if top.Type != types.CategoryTypeNone && top.Type != prior {
		tokens = append(tokens, string(top.Type))
	} else if top.Type != types.CategoryTypeNone && prior == types.CategoryTypeNone {
		tokens = append(tokens, string(top.Type))
	}

	return tokens
}

/*
OpportunityLead is the share of Leads edge weight pointing into opportunity-family
categories versus exhaustion-family categories for one symbol. It is the
symmetric counterpart to ExhaustionLead, used to boost entry utility when the
resident graph shows the current category sequence precedes opportunity regimes.
*/
func (reporter Reporter) OpportunityLead(symbol string) (share float64, dominates bool) {
	if reporter.graph == nil || symbol == "" {
		return 0, false
	}

	graph := reporter.graph
	graph.mu.RLock()
	defer graph.mu.RUnlock()

	var intoExhaustion, intoOpportunity float64

	for _, key := range graph.edgesBySymbol[symbol] {
		relation := graph.EdgeIndex[key]

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

	share = intoOpportunity / total
	dominates = intoOpportunity > intoExhaustion && intoOpportunity > 0

	return share, dominates
}

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

/*
Top returns the strongest composed category from one Thesis symbol bucket. The
caller already read the bucket directly from Thesis.Categories, so no symbol
filter or intermediate regrouping is needed here.
*/
func Top(categories []types.Category) types.Category {
	for _, category := range categories {
		if category.Type != types.CategoryTypeNone {
			return category
		}
	}

	return types.Category{}
}
