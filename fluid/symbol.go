package fluid

import (
	"math"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
)

type FluidSymbol struct {
	pair        asset.Pair
	bids        []market.BookLevel
	asks        []market.BookLevel
	buyPressure float64
	changePct   float64
	pressure    *adaptive.EMA
	spreadBPS   float64
	score       *numeric.Derived
	forecast    *learned.Forecast
}

func NewFluidSymbol(pair asset.Pair) *FluidSymbol {
	return &FluidSymbol{
		pair:     pair,
		pressure: adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		forecast: learned.NewForecast(0.35),
	}
}

func (state *FluidSymbol) Measure() (engine.Measurement, bool) {
	if len(state.bids) == 0 || len(state.asks) == 0 {
		return engine.Measurement{}, false
	}

	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range state.bids {
		bidVolume += level.Volume
	}

	for _, level := range state.asks {
		askVolume += level.Volume
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return engine.Measurement{}, false
	}

	imbalance := (bidVolume - askVolume) / total
	pressure := (state.buyPressure + 1) / 2

	if state.spreadBPS > 0 {
		pressure *= 1 / (1 + state.spreadBPS/100)
	}

	raw, err := state.score.Push(math.Abs(imbalance), pressure)

	if err != nil || raw <= 0 {
		return engine.Measurement{}, false
	}

	confidence := raw * state.forecast.Scale()

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.Flow,
		Source:     fluidSource,
		Regime:     "fluid",
		Reason:     "book_flow",
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
	}, true
}

func (state *FluidSymbol) ApplyFeedback(feedback engine.PredictionFeedback) {
	_, _ = state.forecast.Next(0, feedback.PredictedReturn, feedback.ActualReturn)
}
