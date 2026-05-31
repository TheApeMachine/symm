package depthflow

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/orderbook"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/toxicity"
)

// toxicLevelFilter excludes book levels the toxicity tracker has flagged — large,
// young, near-touch blocks cancelled rather than filled — from the weighted
// imbalance, so a spoofed wall does not skew the read.
func toxicLevelFilter(symbol string) func(price float64) bool {
	return func(price float64) bool {
		return toxicity.IsToxic(symbol, price, time.Now())
	}
}

/*
DepthSymbol owns the per-symbol book/flow state for one DepthFlow consumer and
classifies book shape onto the weight-of-the-book perspective. SNR is each
strength metric scored against its own running noise floor (adaptive.SNRField),
so the reading is in noise-sigma units and comparable to every other signal.

The order book is a maintained orderbook.Book, not the raw last delta: Kraken sends
a snapshot then checksum-verified deltas, and folding each delta into the local book
is what makes the imbalance and spoof reads correct. Reading the delta as if it were
a whole book — the prior bug — discarded every level the delta did not mention.
*/
type DepthSymbol struct {
	mu       sync.RWMutex
	symbol   string
	bookFeed *market.BookFeedState
	last        float64
	bid         float64
	ask         float64
	buyPressure float64
	pressure *adaptive.EMA
	score    *numeric.Derived
}

func NewDepthSymbol(symbol string) *DepthSymbol {
	return &DepthSymbol{
		symbol: symbol,
		bookFeed: market.NewBookFeedState(
			symbol,
			"depthflow",
			config.System.BookDepthLevels,
		),
		pressure: adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
	}
}

/*
ApplyBook folds one book frame into the maintained local book — a snapshot replaces
it, a delta is merged in with zero-qty levels removed — then verifies the exchange
checksum against the result so a divergence is reported rather than fed silently into
the imbalance read.
*/
func (state *DepthSymbol) ApplyBook(update market.BookUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.bookFeed.Apply(update)
}

func (state *DepthSymbol) PushTradePressure(sign float64) (float64, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	value, err := state.pressure.Next(0, sign)

	if err != nil {
		return 0, err
	}

	state.buyPressure = value

	return value, nil
}

func (state *DepthSymbol) HasBook() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.bookFeed.Ready()
}

func (state *DepthSymbol) FeedTicker(row market.TickerUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

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

func (state *DepthSymbol) Measure() (perspectives.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.bookFeed.Diverged() {
		return state.measureTradePressureLocked()
	}

	bids := toMarketLevels(state.bookFeed.Book().Bids())
	asks := toMarketLevels(state.bookFeed.Book().Asks())
	mid := state.last

	if len(bids) > 0 && len(asks) > 0 {
		mid = (bids[0].Price + asks[0].Price) / 2
	}

	if mid <= 0 && state.bid > 0 && state.ask > 0 {
		mid = (state.bid + state.ask) / 2
	}

	if mid <= 0 {
		return perspectives.Measurement{}, false
	}

	if len(bids) > 0 && len(asks) > 0 {
		imbalance, ok := market.WeightedDepthImbalanceFiltered(
			bids, asks, mid, config.System.BookDepthDecayLambda, toxicLevelFilter(state.symbol),
		)
		level1, levelOK := market.Level1Imbalance(bids, asks)

		if ok && imbalance != 0 && levelOK {
			flatImbalance, flatOK := market.FlatDepthImbalance(bids, asks)
			spoofed := market.IsSpoofSkew(
				imbalance, level1, config.System.SpoofWeightedThreshold, config.System.SpoofLevel1Reject,
			)

			if flatOK {
				spoofed = spoofed || market.IsSpoofSkew(
					flatImbalance, level1, config.System.SpoofWeightedThreshold, config.System.SpoofLevel1Reject,
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

				raw, err := state.score.Push(math.Abs(imbalance), pressure)

				if err != nil {
					errnie.Error(err)
				}

				if raw > 0 {
					measurement := perspectives.Measurement{
						Symbol:   state.symbol,
						Source:   perspectives.SourceDepthFlow,
						Category: depthflowCategory(reasonDepthImbalance, imbalance, flatImbalance, flatOK),
					}

					return perspectives.WithGaugeFactors(perspectives.FinalizeMeasurement(
						measurement, raw, "imbalance",
					), []perspectives.GaugeFactor{
						{Name: "imbalance", Value: imbalance},
						{Name: "level1", Value: level1},
					}), true
				}
			}

			raw := math.Abs(level1)

			measurement := perspectives.Measurement{
				Symbol:   state.symbol,
				Source:   perspectives.SourceDepthFlow,
				Category: depthflowCategory(reasonDepthSkeptic, imbalance, flatImbalance, flatOK),
			}

			return perspectives.WithGaugeFactors(perspectives.FinalizeMeasurement(
				measurement, raw, "level1",
			), []perspectives.GaugeFactor{
				{Name: "imbalance", Value: imbalance},
				{Name: "level1", Value: level1},
			}), true
		}
	}

	return state.measureTradePressureLocked()
}

func (state *DepthSymbol) measureTradePressureLocked() (perspectives.Measurement, bool) {
	flow := math.Abs(state.buyPressure)

	if flow <= 0 {
		flow = math.Abs(state.pressure.Value())
	}

	if flow <= 0 {
		return perspectives.Measurement{}, false
	}

	return perspectives.FinalizeMeasurement(perspectives.Measurement{
		Symbol:   state.symbol,
		Source:   perspectives.SourceDepthFlow,
		Category: depthflowCategory("trade_pressure", 0, 0, false),
	}, flow, "flow"), true
}

// toMarketLevels converts maintained-book levels back to the market.BookLevel shape
// the imbalance helpers consume.
func toMarketLevels(levels []orderbook.Level) []market.BookLevel {
	out := make([]market.BookLevel, len(levels))

	for index, level := range levels {
		out[index] = market.BookLevel{Price: level.Price, Qty: level.Qty}
	}

	return out
}
