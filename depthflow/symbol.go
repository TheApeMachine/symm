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
	last        float64
	bid         float64
	ask         float64
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

func (state *DepthSymbol) FeedTicker(row market.TickerRow) {
	if row.Last > 0 {
		state.last = row.Last
	}

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}
}

func (state *DepthSymbol) Measure() (engine.Measurement, bool) {
	bid := state.bid
	ask := state.ask
	mid := state.last

	if len(state.bids) > 0 && len(state.asks) > 0 {
		bid = state.bids[0].Price
		ask = state.asks[0].Price
		mid = (bid + ask) / 2
	}

	if mid <= 0 && bid > 0 && ask > 0 {
		mid = (bid + ask) / 2
	}

	if mid <= 0 {
		return engine.Measurement{}, false
	}

	if len(state.bids) > 0 && len(state.asks) > 0 {
		imbalance, ok := market.WeightedDepthImbalance(
			state.bids,
			state.asks,
			mid,
			config.System.BookDepthDecayLambda,
		)

		level1Imbalance, levelOK := market.Level1Imbalance(state.bids, state.asks)

		if ok && imbalance != 0 && levelOK {
			spoofed := market.IsSpoofSkew(
				imbalance,
				level1Imbalance,
				config.System.SpoofWeightedThreshold,
				config.System.SpoofLevel1Reject,
			)

			flatImbalance, flatOK := market.FlatDepthImbalance(state.bids, state.asks)

			if flatOK {
				spoofed = spoofed || market.IsSpoofSkew(
					flatImbalance,
					level1Imbalance,
					config.System.SpoofWeightedThreshold,
					config.System.SpoofLevel1Reject,
				)
			}

			if !spoofed {
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
				}

				if raw > 0 {
					confidence := engine.ConfidenceFromScore(raw)

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
						Last:       mid,
						Bid:        bid,
						Ask:        ask,
					}, true
				}
			}

			confidence := engine.ConfidenceFromScore(math.Abs(level1Imbalance))

			if confidence > 0 {
				return engine.Measurement{
					Type:       engine.DepthFlow,
					Source:     depthflowSource,
					Regime:     "depth",
					Reason:     "depth_skeptic",
					Pairs:      []asset.Pair{state.pair},
					Confidence: confidence,
					Last:       mid,
					Bid:        bid,
					Ask:        ask,
				}, true
			}
		}
	}

	flow := math.Abs(state.buyPressure)

	if flow <= 0 {
		flow = math.Abs(state.pressure.Value())
	}

	confidence := engine.ConfidenceFromScore(flow)

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.DepthFlow,
		Source:     depthflowSource,
		Regime:     "depth",
		Reason:     "trade_pressure",
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       mid,
		Bid:        bid,
		Ask:        ask,
	}, true
}
