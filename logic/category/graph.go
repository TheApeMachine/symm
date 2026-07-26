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
	nodes         map[nodeKey]*Node
	edges         map[edgeKey]*Relation
	edgesBySymbol map[string][]edgeKey
	prior         map[string]types.CategoryType
	cadence       cadenceBook
	touched       map[edgeKey]struct{}
	previous      map[nodeKey]Node
	// pair tracks cumulative activation mass for independence tests.
	pair *pairMemory
}

/*
NewGraph allocates an empty resident category graph.
*/
func NewGraph() *Graph {
	return &Graph{
		nodes:         map[nodeKey]*Node{},
		edges:         map[edgeKey]*Relation{},
		edgesBySymbol: map[string][]edgeKey{},
		prior:         map[string]types.CategoryType{},
		previous:      map[nodeKey]Node{},
		pair:          newPairMemory(),
	}
}

/*
Update upserts composed category nodes and derives typed edges from
CategoryAffinity plus measurement temporal envelopes on the Thesis. It reuses
the Thesis category buckets so direct graph tests and strategy proofs exercise
the same resident graph commit path without allocating a separate compose API.
*/
func (graph *Graph) Update(
	at time.Time, thesis *types.Thesis, categories []types.Category,
) {
	for symbol, rows := range thesis.Categories {
		thesis.Categories[symbol] = rows[:0]
	}

	for _, category := range categories {
		if category.Symbol == "" {
			continue
		}

		thesis.Categories[category.Symbol] = append(
			thesis.Categories[category.Symbol], category,
		)
	}

	for symbol, rows := range thesis.Categories {
		if len(rows) == 0 {
			delete(thesis.Categories, symbol)
		}
	}

	thesis.At = at
	graph.UpdateFrom(thesis)
}

/*
UpdateFrom upserts composed category nodes and derives typed edges from
CategoryAffinity plus measurement temporal envelopes on a pre-snapshotted
measurement slice. Prior tops are recorded for DMT transition tokens only —
they do not mint Leads/Lags. Only categories touched on this cut snapshot
prior node state, so large resident graphs do not pay a full-map copy on
every tick. The touched map is cleared and reused to avoid per-tick allocation.
*/
func (graph *Graph) UpdateFrom(thesis *types.Thesis) {
	if graph.touched == nil {
		graph.touched = make(map[edgeKey]struct{})
	} else {
		for key := range graph.touched {
			delete(graph.touched, key)
		}
	}

	evidence := indexEvidence(thesis.Measurements)

	for symbol, bySymbol := range thesis.Categories {
		for key := range graph.previous {
			delete(graph.previous, key)
		}

		for _, category := range bySymbol {
			if category.Symbol == "" || category.Type == types.CategoryTypeNone {
				continue
			}

			key := nodeKey{symbol: category.Symbol, kind: category.Type}
			node := graph.nodes[key]

			if node != nil {
				if _, captured := graph.previous[key]; !captured {
					graph.previous[key] = *node
				}
			}

			if node == nil {
				node = &Node{Symbol: category.Symbol, Type: category.Type}
				graph.nodes[key] = node
			}

			node.Strength = category.Strength
			node.Freshness = category.Freshness
			node.At = thesis.At
			graph.pair.observe(category.Symbol, category.Type, category.Strength)
		}

		mean := graph.cadence.touch(symbol, thesis.At)

		for left := 0; left < len(bySymbol); left++ {
			for right := left + 1; right < len(bySymbol); right++ {
				graph.pair.coobserve(
					symbol, bySymbol[left].Type, bySymbol[right].Type,
					bySymbol[left].Strength, bySymbol[right].Strength,
				)
				graph.linkPair(thesis.At, evidence, symbol, bySymbol[left], bySymbol[right])
			}
		}

		graph.linkActivationLeads(thesis, symbol, graph.previous)
		graph.cadence.decayIdle(graph, symbol, thesis.At, mean)
		graph.prior[symbol] = Top(bySymbol).Type
	}
}

/*
linkActivationLeads strengthens Leads/Lags when a category newly activates after
another category was already active on a prior cut for the same symbol. Order
comes from resident node timestamps, not from top-winner label changes.
*/
func (graph *Graph) linkActivationLeads(
	thesis *types.Thesis,
	symbol string,
	previous map[nodeKey]Node,
) {
	for _, category := range thesis.Categories[symbol] {
		key := nodeKey{symbol: symbol, kind: category.Type}
		prior, existed := previous[key]

		if existed && prior.Strength > 0 {
			continue
		}

		if category.Strength <= 0 {
			continue
		}

		for _, peer := range thesis.Categories[symbol] {
			if peer.Type == category.Type || peer.Strength <= 0 {
				continue
			}

			peerKey := nodeKey{symbol: symbol, kind: peer.Type}
			peerPrior, peerExisted := previous[peerKey]

			if !peerExisted || peerPrior.Strength <= 0 {
				continue
			}

			if !peerPrior.At.Before(thesis.At) {
				continue
			}

			mass := math.Sqrt(peer.Strength * category.Strength)
			evidence := append(append([]string{}, peer.Supporting...), category.Supporting...)
			graph.strengthen(thesis.At, symbol, peer.Type, category.Type, Leads, mass, evidence)
			graph.strengthen(thesis.At, symbol, category.Type, peer.Type, Lags, mass, evidence)
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
		graph.edgesBySymbol[symbol] = append(graph.edgesBySymbol[symbol], key)
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
