package pumpdump

import (
	"fmt"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/types"
)

type tickerState struct {
	pipeline *nomagique.Pipeline
	dynamics *equation.AdaptiveZScore
}

/*
Ticker is the executable-touch market entity. It measures displayed capacity,
executable spread, and historical relative-spread baseline using the v2 AdaptiveZScore equation.
Zero Wire blocks, zero Frame allocations, zero imperative math.
*/
type Ticker struct {
	states map[string]*tickerState
	mu     sync.RWMutex
}

func NewTicker() *Ticker {
	return &Ticker{
		states: make(map[string]*tickerState),
	}
}

func (ticker *Ticker) Close() error {
	return nil
}

func (ticker *Ticker) Step(tick kraken.TickerData) *data.Measurement[float64] {
	if tick.Bid == nil || tick.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: ticker requires bid and ask")}
	}

	bid := tick.Bid.Float64()
	ask := tick.Ask.Float64()

	if bid <= 0 || ask <= 0 || ask <= bid {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: positive order violated (%f <= %f)", ask, bid)}
	}

	midpoint := (bid + ask) / 2.0
	spread := ask - bid
	relativeSpread := types.Scalar(spread / midpoint)

	ticker.mu.Lock()
	state, found := ticker.states[tick.Symbol]

	if !found {
		dynamics := &equation.AdaptiveZScore{}
		state = &tickerState{
			dynamics: dynamics,
			pipeline: nomagique.Number(dynamics),
		}
		ticker.states[tick.Symbol] = state
	}

	state.pipeline.Step(relativeSpread)
	dyn := state.dynamics
	ticker.mu.Unlock()

	id := fmt.Sprintf("pumpdump:%s:%d", tick.Symbol, tick.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, tick.Symbol, "pumpdump", tick.Timestamp, tick.Timestamp)
	measurement.Metadata = make(map[string]float64)

	putPumpDumpMetric(measurement, "best_bid", bid, data.UnitRate)
	putPumpDumpMetric(measurement, "best_ask", ask, data.UnitRate)
	putPumpDumpMetric(measurement, "midpoint", midpoint, data.UnitRate)
	putPumpDumpMetric(measurement, "spread", spread, data.UnitRate)
	putPumpDumpMetric(measurement, "relative_spread", float64(relativeSpread), data.UnitDimensionless)

	putPumpDumpMetric(measurement, "relative_spread_baseline", float64(dyn.Baseline()), data.UnitDimensionless)
	putPumpDumpMetric(measurement, "spread_ratio", float64(dyn.Ratio()), data.UnitDimensionless)

	if dyn.HasPrior() {
		putPumpDumpMetric(measurement, "spread_divergence", float64(dyn.Divergence()), data.UnitDimensionless)
		putPumpDumpMetric(measurement, "spread_zscore", float64(dyn.ZScore()), data.UnitDimensionless)
	}

	measurement.Maturity = float64(dyn.Maturity())

	return measurement
}

func putPumpDumpMetric(m *data.Measurement[float64], name string, val float64, unit data.Unit) {
	m.PutMetric(data.NewMetric(
		name, val, nil, nil, unit, data.TimescaleInstantaneous,
	))
}
