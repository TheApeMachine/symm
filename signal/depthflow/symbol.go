package depthflow

import (
	"fmt"
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
	mu          sync.RWMutex
	symbol      string
	book        *orderbook.Book
	diverged    bool
	last        float64
	bid         float64
	ask         float64
	buyPressure float64
	pressure    *adaptive.EMA
	score       *numeric.Derived
	floors      *adaptive.SNRField
}

func NewDepthSymbol(symbol string) *DepthSymbol {
	return &DepthSymbol{
		symbol:   symbol,
		book:     orderbook.NewBook(config.System.BookDepthLevels),
		pressure: adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		floors: adaptive.NewSNRField(),
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

	state.applyFrameLocked(update)
	state.verifyLocked(uint32(update.Checksum))
}

func (state *DepthSymbol) applyFrameLocked(update market.BookUpdate) {
	if update.IsSnapshot() {
		state.book.ApplySnapshot(update.BidLevels(), update.AskLevels())

		return
	}

	state.book.ApplyDelta(update.BidLevels(), update.AskLevels())
}

// verifyLocked compares the maintained book against the exchange checksum, reporting
// a divergence only on the transition into the diverged state so a persistent
// mismatch does not spam the hot path. A divergence means a delta was missed or
// misapplied; the book corrects itself on the next snapshot the feed sends.
func (state *DepthSymbol) verifyLocked(checksum uint32) {
	if checksum == 0 || !state.book.Ready() {
		return
	}

	matches := state.book.Verify(checksum)

	if !matches && !state.diverged {
		errnie.Error(fmt.Errorf("depthflow: book checksum diverged for %s", state.symbol))
	}

	state.diverged = !matches
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

	return state.book.Ready()
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

	bids := toMarketLevels(state.book.Bids())
	asks := toMarketLevels(state.book.Asks())
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
					return perspectives.FinalizeSNR(perspectives.Measurement{
						Source:   perspectives.SourceDepthFlow,
						Category: depthflowCategory(reasonDepthImbalance, imbalance, flatImbalance, flatOK),
					}, raw, func(value float64) float64 {
						return state.floors.Score("imbalance", value)
					}), true
				}
			}

			raw := math.Abs(level1)

			return perspectives.FinalizeSNR(perspectives.Measurement{
				Source:   perspectives.SourceDepthFlow,
				Category: depthflowCategory(reasonDepthSkeptic, imbalance, flatImbalance, flatOK),
			}, raw, func(value float64) float64 {
				return state.floors.Score("level1", value)
			}), true
		}
	}

	flow := math.Abs(state.buyPressure)

	if flow <= 0 {
		flow = math.Abs(state.pressure.Value())
	}

	if flow <= 0 {
		return perspectives.Measurement{}, false
	}

	return perspectives.FinalizeSNR(perspectives.Measurement{
		Source:   perspectives.SourceDepthFlow,
		Category: depthflowCategory("trade_pressure", 0, 0, false),
	}, flow, func(value float64) float64 {
		return state.floors.Score("flow", value)
	}), true
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
