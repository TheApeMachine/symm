package algo

import (
	"math"
	"sort"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

const crossSectionPathLength = 32

/*
CrossSectionNode is the single upstream stage that aggregates market-wide
ticker state and emits one immutable CrossSectionSnapshot per observation.
Every cross-symbol signal consumes this snapshot instead of running its own
universe scan, so breadth, leadership, and cohort statistics are computed
exactly once per tick.

It is the one sanctioned mutex in the streaming pipeline: a shared global
aggregator must serialize writes from every symbol shard. Its output is an
immutable snapshot, so no reader ever observes a partially written view.
*/
type CrossSectionNode struct {
	mu           sync.RWMutex
	lastPrices   map[string]float64
	returns      map[string]float64
	historyPaths map[string][]float64
}

/*
NewCrossSectionNode constructs an empty market-wide cross-section aggregator.
*/
func NewCrossSectionNode() *CrossSectionNode {
	return &CrossSectionNode{
		lastPrices:   make(map[string]float64),
		returns:      make(map[string]float64),
		historyPaths: make(map[string][]float64),
	}
}

/*
Process folds one ticker observation into the cross-section and emits a fresh
snapshot once the symbol has a prior price to compare against.
*/
func (node *CrossSectionNode) Process(
	ticker kraken.TickerData,
) (*types.CrossSectionSnapshot, bool, error) {
	if ticker.Last == nil || ticker.Last.Sign() <= 0 {
		return nil, false, nil
	}

	price := ticker.Last.Float64()
	symbol := ticker.Symbol

	node.mu.Lock()
	defer node.mu.Unlock()

	previous, hasPrevious := node.lastPrices[symbol]
	node.lastPrices[symbol] = price

	if !hasPrevious || previous <= 0 {
		return nil, false, nil
	}

	returnValue := (price - previous) / previous
	node.returns[symbol] = returnValue
	node.historyPaths[symbol] = append(node.historyPaths[symbol], returnValue)

	if len(node.historyPaths[symbol]) > crossSectionPathLength {
		node.historyPaths[symbol] = node.historyPaths[symbol][1:]
	}

	advances, declines, total := 0.0, 0.0, float64(len(node.returns))
	leaderSymbol := ""
	leaderReturn := 0.0
	returns := make([]float64, 0, len(node.returns))
	magnitudes := make([]float64, 0, len(node.returns))

	for peerSymbol, peerReturn := range node.returns {
		if peerReturn > 0 {
			advances++
		} else if peerReturn < 0 {
			declines++
		}

		if math.Abs(peerReturn) > math.Abs(leaderReturn) {
			leaderReturn = peerReturn
			leaderSymbol = peerSymbol
		}

		returns = append(returns, peerReturn)
		magnitudes = append(magnitudes, math.Abs(peerReturn))
	}

	snapshot := &types.CrossSectionSnapshot{
		At:              ticker.Timestamp,
		Breadth:         (advances - declines) / total,
		MedianReturn:    medianFloat(returns),
		MedianMagnitude: medianFloat(magnitudes),
		LeaderSymbol:    leaderSymbol,
		LeaderReturn:    leaderReturn,
		LeaderPath:      append([]float64(nil), node.historyPaths[leaderSymbol]...),
		Returns:         copyReturns(node.returns),
	}

	return snapshot, true, nil
}

func copyReturns(returns map[string]float64) map[string]float64 {
	copied := make(map[string]float64, len(returns))

	for symbol, value := range returns {
		copied[symbol] = value
	}

	return copied
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2

	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}

	return sorted[middle]
}
