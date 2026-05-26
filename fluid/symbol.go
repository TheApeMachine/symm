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
	volume      float64
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

	scaledPressure := pressure * state.forecast.Scale()
	raw, err := state.score.Push(math.Abs(imbalance), scaledPressure)

	if err != nil || raw <= 0 {
		return engine.Measurement{}, false
	}

	confidence := engine.AlignConfidence(math.Abs(imbalance), scaledPressure)

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

func (state *FluidSymbol) wireRow() map[string]any {
	if len(state.bids) == 0 || len(state.asks) == 0 {
		return nil
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
		return nil
	}

	imbalance := (bidVolume - askVolume) / total
	pressure := (state.buyPressure + 1) / 2
	visc := 1 / (1 + state.spreadBPS/100)
	re := math.Max(math.Abs(imbalance), math.Abs(pressure)) * state.forecast.Scale()

	return WireRow(map[string]any{
		"symbol":     state.pair.Wsname,
		"change_pct": state.changePct,
		"vol":        state.volume,
		"div":        imbalance,
		"vort":       state.buyPressure,
		"turb":       pressure * state.spreadBPS / 100,
		"visc":       visc,
		"re":         re,
	})
}
