package exhaust

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/ring"
	"github.com/theapemachine/symm/stats"
)

const exitHistoryCap = 24

/*
symbolHistory holds rolling microstructure samples for exit scoring.
*/
type symbolHistory struct {
	bidDepths  ring.FloatRing
	askDepths  ring.FloatRing
	densities  ring.FloatRing
	spreads    ring.FloatRing
	pressures  ring.FloatRing
	imbalances ring.FloatRing
	lastPrice  float64
	hasLast    bool
}

/*
historyStore accumulates per-symbol exit features between rescore ticks.
*/
type historyStore struct {
	mu       sync.RWMutex
	bySymbol map[string]*symbolHistory
}

func newHistoryStore() *historyStore {
	return &historyStore{
		bySymbol: make(map[string]*symbolHistory),
	}
}

func (store *historyStore) observe(
	symbol string,
	bidDepth, askDepth, density, spreadBPS, buyPressure, imbalance, last float64,
) {
	if symbol == "" {
		return
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	history := store.ensureLocked(symbol)

	if bidDepth > 0 {
		history.bidDepths.Push(bidDepth)
	}

	if askDepth > 0 {
		history.askDepths.Push(askDepth)
	}

	if density > 0 {
		history.densities.Push(density)
	}

	if spreadBPS > 0 {
		history.spreads.Push(spreadBPS)
	}

	if buyPressure != 0 {
		history.pressures.Push(buyPressure)
	}

	if imbalance != 0 {
		history.imbalances.Push(imbalance)
	}

	if last > 0 {
		history.lastPrice = last
		history.hasLast = true
	}
}

func (store *historyStore) snapshot(symbol string) (symbolHistory, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	history, ok := store.bySymbol[symbol]

	if !ok || history == nil {
		return symbolHistory{}, false
	}

	return history.snapshot(), true
}

func (store *historyStore) ensureLocked(symbol string) *symbolHistory {
	history, ok := store.bySymbol[symbol]

	if ok {
		return history
	}

	history = &symbolHistory{
		bidDepths:  ring.NewFloatRing(exitHistoryCap),
		askDepths:  ring.NewFloatRing(exitHistoryCap),
		densities:  ring.NewFloatRing(exitHistoryCap),
		spreads:    ring.NewFloatRing(exitHistoryCap),
		pressures:  ring.NewFloatRing(exitHistoryCap),
		imbalances: ring.NewFloatRing(exitHistoryCap),
	}
	store.bySymbol[symbol] = history

	return history
}

func (history *symbolHistory) snapshot() symbolHistory {
	return symbolHistory{
		bidDepths:  history.bidDepths,
		askDepths:  history.askDepths,
		densities:  history.densities,
		spreads:    history.spreads,
		pressures:  history.pressures,
		imbalances: history.imbalances,
		lastPrice:  history.lastPrice,
		hasLast:    history.hasLast,
	}
}

func depthTrend(depths ring.FloatRing) float64 {
	if depths.Len() < 4 {
		return 0
	}

	ordered := depths.Ordered()
	recent := stats.Mean(ordered[len(ordered)-3:])
	prior := stats.Mean(ordered[:len(ordered)-3])

	if prior <= 0 {
		return 0
	}

	return (prior - recent) / prior
}

func spreadWiden(spreads ring.FloatRing) float64 {
	if spreads.Len() < 4 {
		return 0
	}

	ordered := spreads.Ordered()
	sorted := stats.CopySorted(ordered)
	median := stats.PercentileSorted(sorted, 0.5)
	current := ordered[len(ordered)-1]

	if median <= 0 || current <= median {
		return 0
	}

	return (current - median) / median
}

func pressureFade(pressures ring.FloatRing, side int) float64 {
	if pressures.Len() < 3 {
		return 0
	}

	ordered := pressures.Ordered()
	recent := ordered[len(ordered)-1]
	priorPeak := stats.Max(ordered[:len(ordered)-1])

	if side > 0 {
		if priorPeak <= 0 {
			return 0
		}

		if recent >= priorPeak {
			return 0
		}

		return (priorPeak - recent) / math.Max(math.Abs(priorPeak), 1e-9)
	}

	if priorPeak >= 0 {
		return 0
	}

	if recent <= priorPeak {
		return 0
	}

	return (recent - priorPeak) / math.Max(math.Abs(priorPeak), 1e-9)
}

func imbalanceFlip(imbalances ring.FloatRing, side int) float64 {
	if imbalances.Len() < 2 {
		return 0
	}

	ordered := imbalances.Ordered()
	recent := ordered[len(ordered)-1]
	prior := stats.Mean(ordered[:len(ordered)-1])

	if side > 0 && prior > 0 && recent < 0 {
		return math.Min(1, math.Abs(recent)/math.Max(prior, 1e-9))
	}

	if side < 0 && prior < 0 && recent > 0 {
		return math.Min(1, recent/math.Max(math.Abs(prior), 1e-9))
	}

	return 0
}
