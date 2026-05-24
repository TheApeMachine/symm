package exhaust

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/stats"
)

const exitHistoryCap = 24

/*
symbolHistory holds rolling microstructure samples for exit scoring.
*/
type symbolHistory struct {
	bidDepths  []float64
	askDepths  []float64
	densities  []float64
	spreads    []float64
	pressures  []float64
	imbalances []float64
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
		history.bidDepths = append(history.bidDepths, bidDepth)
	}

	if askDepth > 0 {
		history.askDepths = append(history.askDepths, askDepth)
	}

	if density > 0 {
		history.densities = append(history.densities, density)
	}

	if spreadBPS > 0 {
		history.spreads = append(history.spreads, spreadBPS)
	}

	if buyPressure != 0 {
		history.pressures = append(history.pressures, buyPressure)
	}

	if imbalance != 0 {
		history.imbalances = append(history.imbalances, imbalance)
	}

	if last > 0 {
		history.lastPrice = last
		history.hasLast = true
	}

	history.trim()
}

func (store *historyStore) snapshot(symbol string) (symbolHistory, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	history, ok := store.bySymbol[symbol]

	if !ok || history == nil {
		return symbolHistory{}, false
	}

	return *history, true
}

func (store *historyStore) ensureLocked(symbol string) *symbolHistory {
	history, ok := store.bySymbol[symbol]

	if ok {
		return history
	}

	history = &symbolHistory{}
	store.bySymbol[symbol] = history

	return history
}

func (history *symbolHistory) trim() {
	history.bidDepths = trimTail(history.bidDepths, exitHistoryCap)
	history.askDepths = trimTail(history.askDepths, exitHistoryCap)
	history.densities = trimTail(history.densities, exitHistoryCap)
	history.spreads = trimTail(history.spreads, exitHistoryCap)
	history.pressures = trimTail(history.pressures, exitHistoryCap)
	history.imbalances = trimTail(history.imbalances, exitHistoryCap)
}

func trimTail(values []float64, cap int) []float64 {
	if len(values) <= cap {
		return values
	}

	return values[len(values)-cap:]
}

func depthTrend(depths []float64) float64 {
	if len(depths) < 4 {
		return 0
	}

	recent := stats.Mean(depths[len(depths)-3:])
	prior := stats.Mean(depths[:len(depths)-3])

	if prior <= 0 {
		return 0
	}

	return (prior - recent) / prior
}

func spreadWiden(spreads []float64) float64 {
	if len(spreads) < 4 {
		return 0
	}

	sorted := stats.CopySorted(spreads)
	median := stats.PercentileSorted(sorted, 0.5)
	current := spreads[len(spreads)-1]

	if median <= 0 || current <= median {
		return 0
	}

	return (current - median) / median
}

func pressureFade(pressures []float64, side int) float64 {
	if len(pressures) < 3 {
		return 0
	}

	recent := pressures[len(pressures)-1]
	priorPeak := stats.Max(pressures[:len(pressures)-1])

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

func imbalanceFlip(imbalances []float64, side int) float64 {
	if len(imbalances) < 2 {
		return 0
	}

	recent := imbalances[len(imbalances)-1]
	prior := stats.Mean(imbalances[:len(imbalances)-1])

	if side > 0 && prior > 0 && recent < 0 {
		return math.Min(1, math.Abs(recent)/math.Max(prior, 1e-9))
	}

	if side < 0 && prior < 0 && recent > 0 {
		return math.Min(1, recent/math.Max(math.Abs(prior), 1e-9))
	}

	return 0
}
