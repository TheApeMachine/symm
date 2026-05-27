package depthflow

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
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
}

func NewDepthSymbol(pair asset.Pair) *DepthSymbol {
	return &DepthSymbol{
		pair:     pair,
		pressure: adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		forecast: learned.NewForecast(0.35),
	}
}

func (state *DepthSymbol) Measure() (engine.Measurement, bool) {
	if len(state.bids) == 0 || len(state.asks) == 0 {
		return engine.Measurement{}, false
	}

	bid := state.bids[0].Price
	ask := state.asks[0].Price
	mid := (bid + ask) / 2

	if mid <= 0 {
		return engine.Measurement{}, false
	}

	imbalance, ok := market.WeightedDepthImbalance(
		state.bids,
		state.asks,
		mid,
		config.System.BookDepthDecayLambda,
	)

	if !ok || imbalance == 0 {
		return engine.Measurement{}, false
	}

	level1Imbalance, ok := market.Level1Imbalance(state.bids, state.asks)

	if !ok {
		return engine.Measurement{}, false
	}

	if market.IsSpoofSkew(
		imbalance,
		level1Imbalance,
		config.System.SpoofWeightedThreshold,
		config.System.SpoofLevel1Reject,
	) {
		return engine.Measurement{}, false
	}

	flatImbalance, flatOK := market.FlatDepthImbalance(state.bids, state.asks)

	if flatOK && market.IsSpoofSkew(
		flatImbalance,
		level1Imbalance,
		config.System.SpoofWeightedThreshold,
		config.System.SpoofLevel1Reject,
	) {
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

	if err != nil {
		errnie.Error(err)
		return engine.Measurement{}, false
	}

	if raw <= 0 {
		return engine.Measurement{}, false
	}

	confidence := engine.AlignConfidence(math.Abs(imbalance), pressure*state.forecast.Scale())

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	last := mid

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
		Last:       last,
		Bid:        bid,
		Ask:        ask,
	}, true
}
