package depthflow

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
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
*/
type DepthSymbol struct {
	mu          sync.RWMutex
	symbol      string
	bids        []market.BookLevel
	asks        []market.BookLevel
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
		pressure: adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		floors: adaptive.NewSNRField(),
	}
}

func (state *DepthSymbol) SetBook(bids, asks []market.BookLevel) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(bids) > 0 {
		state.bids = append(state.bids[:0:0], bids...)
	}

	if len(asks) > 0 {
		state.asks = append(state.asks[:0:0], asks...)
	}
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

	return len(state.bids) > 0 && len(state.asks) > 0
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

	bids := state.bids
	asks := state.asks
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
					return perspectives.Measurement{
						Source:   perspectives.SourceDepthFlow,
						Category: depthflowCategory(reasonDepthImbalance, imbalance, flatImbalance, flatOK),
						SNR:      state.floors.Score("imbalance", raw),
					}, true
				}
			}

			return perspectives.Measurement{
				Source:   perspectives.SourceDepthFlow,
				Category: depthflowCategory(reasonDepthSkeptic, imbalance, flatImbalance, flatOK),
				SNR:      state.floors.Score("level1", math.Abs(level1)),
			}, true
		}
	}

	flow := math.Abs(state.buyPressure)

	if flow <= 0 {
		flow = math.Abs(state.pressure.Value())
	}

	if flow <= 0 {
		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Source:   perspectives.SourceDepthFlow,
		Category: depthflowCategory("trade_pressure", 0, 0, false),
		SNR:      state.floors.Score("flow", flow),
	}, true
}
