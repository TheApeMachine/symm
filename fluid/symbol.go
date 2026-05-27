package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
)

type FluidSymbol struct {
	pair            asset.Pair
	bids            []market.BookLevel
	asks            []market.BookLevel
	prevBids        []market.BookLevel
	prevAsks        []market.BookLevel
	buyPressure     float64
	changePct       float64
	volume          float64
	last            float64
	bid             float64
	ask             float64
	pressure        *adaptive.EMA
	spreadBPS       float64
	bookFluxWindow  *adaptive.Window
	tradeFluxWindow *adaptive.Window
	score           *numeric.Derived
	forecast        *learned.Forecast
}

func NewFluidSymbol(pair asset.Pair) *FluidSymbol {
	return &FluidSymbol{
		pair:            pair,
		pressure:        adaptive.NewEMA(0),
		bookFluxWindow:  adaptive.NewWindow(config.System.BookFluxWindow),
		tradeFluxWindow: adaptive.NewWindow(config.System.BookFluxWindow),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		forecast: learned.NewForecast(0.35),
	}
}

func (state *FluidSymbol) FeedBook(delta market.BookLevelsDelta) {
	flux := 0.0

	if len(state.prevBids) > 0 || len(state.prevAsks) > 0 {
		if delta.BidOK {
			flux += sideChangeFlux(state.prevBids, delta.Bids)
		}

		if delta.AskOK {
			flux += sideChangeFlux(state.prevAsks, delta.Asks)
		}
	}

	if delta.BidOK {
		state.prevBids = append([]market.BookLevel(nil), delta.Bids...)
	}

	if delta.AskOK {
		state.prevAsks = append([]market.BookLevel(nil), delta.Asks...)
	}

	if flux <= 0 {
		return
	}

	if _, err := state.bookFluxWindow.Next(0, float64(time.Now().UnixNano()), flux); err != nil {
		errnie.Error(err)
	}
}

func sideChangeFlux(previous, updated []market.BookLevel) float64 {
	previousByPrice := make(map[float64]float64, len(previous))

	for _, level := range previous {
		previousByPrice[level.Price] = level.Volume
	}

	flux := 0.0
	seen := make(map[float64]bool, len(updated))

	for _, level := range updated {
		flux += math.Abs(level.Volume - previousByPrice[level.Price])
		seen[level.Price] = true
	}

	for price, volume := range previousByPrice {
		if seen[price] {
			continue
		}

		flux += volume
	}

	return flux
}

func (state *FluidSymbol) FeedTrade(at time.Time, qty float64) {
	if qty <= 0 {
		return
	}

	if _, err := state.tradeFluxWindow.Next(0, float64(at.UnixNano()), qty); err != nil {
		errnie.Error(err)
	}
}

func (state *FluidSymbol) bookFluxTrustworthy() bool {
	bookFlux := state.bookFluxWindow.Sum()
	tradeFlux := state.tradeFluxWindow.Sum()

	if bookFlux <= 0 {
		return true
	}

	return tradeFlux/bookFlux >= config.System.MinFillToCancelRatio
}

func (state *FluidSymbol) Measure() (engine.Measurement, bool) {
	row := state.wireRow()

	if row == nil {
		return engine.Measurement{}, false
	}

	re, ok := row["re"].(float64)

	if !ok || re <= 0 {
		return engine.Measurement{}, false
	}

	bid := 0.0
	ask := 0.0
	mid := 0.0

	if len(state.bids) > 0 && len(state.asks) > 0 {
		bid = state.bids[0].Price
		ask = state.asks[0].Price
		mid = (bid + ask) / 2
	}

	if mid <= 0 && state.last > 0 {
		bid = state.bid
		ask = state.ask

		if bid <= 0 {
			bid = state.last
		}

		if ask <= 0 {
			ask = state.last
		}

		mid = state.last

		if bid > 0 && ask > 0 {
			mid = (bid + ask) / 2
		}
	}

	if mid <= 0 {
		return engine.Measurement{}, false
	}

	confidence := engine.ConfidenceFromScore(re)
	reason := "field_activity"

	if state.bookFluxTrustworthy() {
		imbalance, imbalanceOK := market.WeightedDepthImbalance(
			state.bids,
			state.asks,
			mid,
			config.System.BookDepthDecayLambda,
		)

		if imbalanceOK && imbalance != 0 {
			level1Imbalance, level1OK := market.Level1Imbalance(state.bids, state.asks)

			if level1OK && !market.IsSpoofSkew(
				imbalance,
				level1Imbalance,
				config.System.SpoofWeightedThreshold,
				config.System.SpoofLevel1Reject,
			) {
				flatImbalance, flatOK := market.FlatDepthImbalance(state.bids, state.asks)

				if !flatOK || !market.IsSpoofSkew(
					flatImbalance,
					level1Imbalance,
					config.System.SpoofWeightedThreshold,
					config.System.SpoofLevel1Reject,
				) {
					pressure := (state.buyPressure + 1) / 2

					if state.spreadBPS > 0 {
						pressure *= 1 / (1 + state.spreadBPS/100)
					}

					raw, err := state.score.Push(
						math.Abs(imbalance),
						pressure*state.forecast.Scale(),
					)

					if err != nil {
						errnie.Error(err)
					}

					if raw > 0 {
						confidence = engine.ConfidenceFromScore(raw)
						reason = "book_flow"
					}
				}
			}
		}
	}

	return engine.Measurement{
		Type:       engine.Flow,
		Source:     fluidSource,
		Regime:     "fluid",
		Reason:     reason,
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       mid,
		Bid:        bid,
		Ask:        ask,
	}, true
}

func (state *FluidSymbol) ApplyFeedback(feedback engine.PredictionFeedback) {
	if _, err := state.forecast.Next(
		0, feedback.PredictedReturn, feedback.ActualReturn,
	); err != nil {
		errnie.Error(err)
	}
}

func (state *FluidSymbol) wireRow() map[string]any {
	imbalance := 0.0
	pressure := (state.buyPressure + 1) / 2
	visc := 1 / (1 + state.spreadBPS/100)

	if len(state.bids) > 0 && len(state.asks) > 0 {
		bidVolume := 0.0
		askVolume := 0.0

		for _, level := range state.bids {
			bidVolume += level.Volume
		}

		for _, level := range state.asks {
			askVolume += level.Volume
		}

		total := bidVolume + askVolume

		if total > 0 {
			imbalance = (bidVolume - askVolume) / total
		}
	}

	if state.volume <= 0 && state.changePct == 0 && imbalance == 0 && pressure == 0.5 {
		return nil
	}

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
