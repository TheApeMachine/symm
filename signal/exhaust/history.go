package exhaust

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/ring"
)

const exitHistoryCap = 24

/*
symbolHistory holds rolling microstructure samples for exit scoring.
*/
type symbolHistory struct {
	bidDepths   ring.FloatRing
	askDepths   ring.FloatRing
	densities   ring.FloatRing
	spreads     ring.FloatRing
	pressures   ring.FloatRing
	pressureEMA *adaptive.EMA
	imbalances  ring.FloatRing
	lastPrice   float64
	hasLast     bool
	tracked     *types.Category
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
		// buyPressure arrives as the raw trade sign (±1). Storing that directly made
		// the pressures ring binary, so pressureFade read (priorPeak − recent)/peak =
		// (1 − −1)/1 = 2 on every sell-after-buy — a degenerate constant that always
		// won the exit vote. Smooth the signed flow into a continuous net-pressure
		// trajectory (the EMA derives its own rate from the flow's volatility) so the
		// ring captures buy pressure genuinely building and fading.
		smoothed, _ := history.pressureEMA.Next(0, buyPressure)

		history.pressures.Push(smoothed)
	}

	if imbalance != 0 {
		history.imbalances.Push(imbalance)
	}

	if last > 0 {
		history.lastPrice = last
		history.hasLast = true
	}
}

func (store *historyStore) measure(symbol string) (types.Measurement, float64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	history, ok := store.bySymbol[symbol]

	if !ok || history == nil {
		return types.Measurement{}, 0, nil
	}

	if !history.hasLast || history.lastPrice <= 0 {
		return types.Measurement{}, 0, nil
	}

	measurement, standout, err := exhaustMeasurement(history.snapshot(), history.tracked)

	if err != nil || measurement.Source == types.SourceNone {
		return measurement, standout, err
	}

	measurement.Last = history.lastPrice

	return measurement, standout, nil
}

func (store *historyStore) ensureLocked(symbol string) *symbolHistory {
	history, ok := store.bySymbol[symbol]

	if ok {
		return history
	}

	history = &symbolHistory{
		bidDepths:   ring.NewFloatRing(exitHistoryCap),
		askDepths:   ring.NewFloatRing(exitHistoryCap),
		densities:   ring.NewFloatRing(exitHistoryCap),
		spreads:     ring.NewFloatRing(exitHistoryCap),
		pressures:   ring.NewFloatRing(exitHistoryCap),
		pressureEMA: adaptive.NewEMA(0),
		imbalances:  ring.NewFloatRing(exitHistoryCap),
		tracked:     types.NewCategory(types.CategoryTypeNone),
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
	recent := numeric.Mean(ordered[len(ordered)-3:])
	prior := numeric.Mean(ordered[:len(ordered)-3])

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
	sorted := numeric.CopySorted(ordered)
	median := numeric.PercentileSorted(sorted, 0.5)
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
	priorPeak := numeric.Max(ordered[:len(ordered)-1])

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
	prior := numeric.Mean(ordered[:len(ordered)-1])

	if side > 0 && prior > 0 && recent < 0 {
		return types.UnitCompetitionMargin(math.Abs(recent), math.Max(prior, 1e-9))
	}

	if side < 0 && prior < 0 && recent > 0 {
		return types.UnitCompetitionMargin(recent, math.Max(math.Abs(prior), 1e-9))
	}

	return 0
}
