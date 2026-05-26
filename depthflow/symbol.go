package depthflow

import (
	"math"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
	"github.com/theapemachine/symm/numeric/logic"
)

type DepthSymbol struct {
	pair        asset.Pair
	bids        []market.BookLevel
	asks        []market.BookLevel
	buyPressure float64
	pressure    *adaptive.EMA
	score       *numeric.Derived
	forecast    *learned.Forecast
	confidence  *engine.SymbolConfidence
}

func NewDepthSymbol(pair asset.Pair) *DepthSymbol {
	return &DepthSymbol{
		pair:     pair,
		pressure: adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		forecast:   learned.NewForecast(0.35),
		confidence: engine.NewSymbolConfidence(engine.DefaultCalibrationParams()),
	}
}

func (state *DepthSymbol) Measure() (engine.Measurement, bool) {
	if len(state.bids) == 0 || len(state.asks) == 0 {
		return engine.Measurement{}, false
	}

	bidVolume := 0.0

	for _, level := range state.bids {
		bidVolume += level.Volume
	}

	askVolume := 0.0

	for _, level := range state.asks {
		askVolume += level.Volume
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return engine.Measurement{}, false
	}

	imbalance := (bidVolume - askVolume) / total

	if imbalance == 0 {
		return engine.Measurement{}, false
	}

	pressure := 1.0

	if state.buyPressure > 0 && imbalance > 0 {
		pressure = (state.buyPressure + 1) / 2
	}

	if state.buyPressure < 0 && imbalance < 0 {
		pressure = (1 - state.buyPressure) / 2
	}

	raw, err := state.score.Push(math.Abs(imbalance), pressure*state.forecast.Scale())

	if err != nil || raw <= 0 {
		return engine.Measurement{}, false
	}

	confidence, ok := state.confidence.Measure(raw)

	if !ok {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type: logic.Or(
			engine.Dump,
			engine.DepthFlow,
			imbalance < 0,
		),
		Source:     depthflowSource,
		Regime:     "depth",
		Reason:     "depth_imbalance",
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
	}, true
}
