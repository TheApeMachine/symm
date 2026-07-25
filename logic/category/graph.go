package category

import (
	"math"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
Graph is the single resident category network for the process lifetime. Nodes
and typed edges are upserted each cut; weights only strengthen on evidence —
the graph is never rebuilt from scratch on a tick.
*/
type Graph struct {
	nodes   map[nodeKey]*Node
	edges   map[edgeKey]*Relation
	prior   map[string]types.CategoryType
	cadence map[string]*symbolCadence
	touched map[edgeKey]struct{}
	// pair tracks cumulative activation mass for independence tests.
	pair *pairMemory
}

/*
NewGraph allocates an empty resident category graph.
*/
func NewGraph() *Graph {
	return &Graph{
		nodes:   map[nodeKey]*Node{},
		edges:   map[edgeKey]*Relation{},
		prior:   map[string]types.CategoryType{},
		cadence: map[string]*symbolCadence{},
		pair:    newPairMemory(),
	}
}

/*
Update upserts composed category nodes and derives typed edges from
CategoryAffinity plus measurement temporal envelopes on the Thesis. Prior tops
are recorded for DMT transition tokens only — they do not mint Leads/Lags.
*/
func (graph *Graph) Update(
	at time.Time, thesis *types.Thesis, categories []types.Category,
) {
	if graph == nil {
		return
	}

	graph.touched = map[edgeKey]struct{}{}
	previous := map[nodeKey]Node{}

	for key, node := range graph.nodes {
		if node == nil {
			continue
		}

		previous[key] = *node
	}

	bySymbol := map[string][]types.Category{}
	evidence := indexEvidence(thesis)

	for _, category := range categories {
		if category.Symbol == "" || category.Type == types.CategoryTypeNone {
			continue
		}

		key := nodeKey{symbol: category.Symbol, kind: category.Type}
		node := graph.nodes[key]

		if node == nil {
			node = &Node{Symbol: category.Symbol, Type: category.Type}
			graph.nodes[key] = node
		}

		node.Strength = category.Strength
		node.Freshness = category.Freshness
		node.At = at
		bySymbol[category.Symbol] = append(bySymbol[category.Symbol], category)
		graph.pair.observe(category.Symbol, category.Type, category.Strength)
	}

	for symbol, active := range bySymbol {
		mean := graph.touchCadence(symbol, at)

		for left := 0; left < len(active); left++ {
			for right := left + 1; right < len(active); right++ {
				graph.pair.coobserve(
					symbol, active[left].Type, active[right].Type,
					active[left].Strength, active[right].Strength,
				)
				graph.linkPair(at, evidence, symbol, active[left], active[right])
			}
		}

		graph.linkActivationLeads(at, symbol, active, previous)
		graph.decayIdle(symbol, at, mean)
		graph.prior[symbol] = Top(active, symbol).Type
	}
}

/*
linkActivationLeads strengthens Leads/Lags when a category newly activates after
another category was already active on a prior cut for the same symbol. Order
comes from resident node timestamps, not from top-winner label changes.
*/
func (graph *Graph) linkActivationLeads(
	at time.Time,
	symbol string,
	active []types.Category,
	previous map[nodeKey]Node,
) {
	for _, category := range active {
		key := nodeKey{symbol: symbol, kind: category.Type}
		prior, existed := previous[key]

		if existed && prior.Strength > 0 {
			continue
		}

		if category.Strength <= 0 {
			continue
		}

		for _, peer := range active {
			if peer.Type == category.Type || peer.Strength <= 0 {
				continue
			}

			peerKey := nodeKey{symbol: symbol, kind: peer.Type}
			peerPrior, peerExisted := previous[peerKey]

			if !peerExisted || peerPrior.Strength <= 0 {
				continue
			}

			if !peerPrior.At.Before(at) {
				continue
			}

			mass := math.Sqrt(peer.Strength * category.Strength)
			evidence := append(append([]string{}, peer.Supporting...), category.Supporting...)
			graph.strengthen(at, symbol, peer.Type, category.Type, Leads, mass, evidence)
			graph.strengthen(at, symbol, category.Type, peer.Type, Lags, mass, evidence)
		}
	}
}

/*
strengthen upserts a typed edge and adds mass to its weight.
*/
func (graph *Graph) strengthen(
	at time.Time,
	symbol string,
	from, to types.CategoryType,
	kind RelationType,
	mass float64,
	evidence []string,
) {
	key := edgeKey{symbol: symbol, from: from, to: to, kind: kind}
	relation := graph.edges[key]

	if relation == nil {
		relation = &Relation{
			Symbol: symbol,
			From:   from,
			To:     to,
			Type:   kind,
		}
		graph.edges[key] = relation
	}

	relation.Weight += mass
	relation.At = at
	relation.Evidence = evidence

	if graph.touched == nil {
		graph.touched = map[edgeKey]struct{}{}
	}

	graph.touched[key] = struct{}{}
}

/*
Weight returns the current weight of a typed edge, or zero when absent.
*/
func (graph *Graph) Weight(
	symbol string,
	from, to types.CategoryType,
	kind RelationType,
) float64 {
	if graph == nil {
		return 0
	}

	relation := graph.edges[edgeKey{symbol: symbol, from: from, to: to, kind: kind}]

	if relation == nil {
		return 0
	}

	return relation.Weight
}

/*
Prior returns the last top category for symbol.
*/
func (graph *Graph) Prior(symbol string) types.CategoryType {
	if graph == nil {
		return types.CategoryTypeNone
	}

	return graph.prior[symbol]
}

/*
TrapPressure is trap vs opportunity category strength on the resident nodes for
symbol, plus Contradicts edge mass that affinity placed from trap→opportunity.
*/
func (graph *Graph) TrapPressure(symbol string) (share float64, dominates bool) {
	if graph == nil || symbol == "" {
		return 0, false
	}

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

	for key, relation := range graph.edges {
		if key.symbol != symbol || relation == nil || relation.Weight <= 0 {
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
Tokens builds the DMT category bag for symbol from the latest composed rows,
including a transition token when the resident prior differs from the current
top.
*/
func (graph *Graph) Tokens(
	symbol string, categories []types.Category,
) []string {
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
