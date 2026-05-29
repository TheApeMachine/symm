package depthflow

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
	"github.com/theapemachine/symm/numeric/logic"
	"github.com/theapemachine/symm/toxicity"
)

// toxicLevelFilter returns a per-level predicate that excludes book levels the
// toxicity subsystem has flagged — large, young, near-touch blocks cancelled
// rather than filled — from the weighted imbalance (§16.3). It is cheap and
// returns false for every level until the toxicity tracker flags one.
func toxicLevelFilter(symbol string) func(price float64) bool {
	return func(price float64) bool {
		return toxicity.IsToxic(symbol, price, time.Now())
	}
}

/*
DepthSymbol owns the mutable per-symbol state for one DepthFlow consumer.

Concurrency. The signal layer mutates bids / asks / last / bid / ask /
buyPressure from the book and tick consumer goroutines while the
publisher reads them through Measure(). mu is per-symbol so cross-symbol
ingest stays parallel; within one symbol the critical sections are
microsecond field updates and a slice-header swap. The slice-header swap
allocates a fresh backing array so a reader that holds the old header is
not racing with append into the same backing memory.
*/
type DepthSymbol struct {
	mu          sync.RWMutex
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

/*
SetBook replaces the depth slices with fresh copies so concurrent readers
do not see torn updates. The slice-header swap is atomic from the
reader's perspective because mu serializes against Measure().
*/
func (state *DepthSymbol) SetBook(bids, asks []market.BookLevel) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(bids) > 0 {
		copyBids := make([]market.BookLevel, len(bids))
		copy(copyBids, bids)
		state.bids = copyBids
	}

	if len(asks) > 0 {
		copyAsks := make([]market.BookLevel, len(asks))
		copy(copyAsks, asks)
		state.asks = copyAsks
	}
}

/*
PushTradePressure advances the EMA under the same mutex Measure() takes
when it reads PressureValue(). adaptive.EMA carries internal running
state, so an unsynchronized Next-vs-Value pair is a data race on the
EMA's fields. The buyPressure scalar is updated in the same critical
section so the EMA tick and the scalar can be read consistently.
*/
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

/*
PressureValue returns the current EMA reading under the read lock so
Measure() observes a consistent snapshot relative to PushTradePressure.
*/
func (state *DepthSymbol) PressureValue() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.pressure.Value()
}

/*
HasBook reports whether at least one bid and one ask have been recorded
under the read lock.
*/
func (state *DepthSymbol) HasBook() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return len(state.bids) > 0 && len(state.asks) > 0
}

func selectIfOK(levels []market.BookLevel, ok bool) []market.BookLevel {
	if !ok {
		return nil
	}

	return levels
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

func (state *DepthSymbol) Measure() (engine.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	bid := state.bid
	ask := state.ask
	mid := state.last
	bids := state.bids
	asks := state.asks
	buyPressure := state.buyPressure

	if len(bids) > 0 && len(asks) > 0 {
		bid = bids[0].Price
		ask = asks[0].Price
		mid = (bid + ask) / 2
	}

	if mid <= 0 && bid > 0 && ask > 0 {
		mid = (bid + ask) / 2
	}

	if mid <= 0 {
		return engine.Measurement{}, false
	}

	if len(bids) > 0 && len(asks) > 0 {
		imbalance, ok := market.WeightedDepthImbalanceFiltered(
			bids,
			asks,
			mid,
			config.System.BookDepthDecayLambda,
			toxicLevelFilter(state.pair.Wsname),
		)

		level1Imbalance, levelOK := market.Level1Imbalance(bids, asks)

		if ok && imbalance != 0 && levelOK {
			spoofed := market.IsSpoofSkew(
				imbalance,
				level1Imbalance,
				config.System.SpoofWeightedThreshold,
				config.System.SpoofLevel1Reject,
			)

			flatImbalance, flatOK := market.FlatDepthImbalance(bids, asks)

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

				if buyPressure > 0 && imbalance > 0 {
					pressure = (buyPressure + 1) / 2
				}

				if buyPressure < 0 && imbalance < 0 {
					pressure = (1 - buyPressure) / 2
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
						Source: depthflowSource,
						Regime: "depth",
						Reason: "depth_imbalance",
						Category: depthflowCategory(
							"depth_imbalance", imbalance, flatImbalance, flatOK,
						),
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
					Type:   engine.DepthFlow,
					Source: depthflowSource,
					Regime: "depth",
					Reason: "depth_skeptic",
					Category: depthflowCategory(
						"depth_skeptic", imbalance, flatImbalance, flatOK,
					),
					Pairs:      []asset.Pair{state.pair},
					Confidence: confidence,
					Last:       mid,
					Bid:        bid,
					Ask:        ask,
				}, true
			}
		}
	}

	flow := math.Abs(buyPressure)

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
		Category:   depthflowCategory("trade_pressure", 0, 0, false),
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       mid,
		Bid:        bid,
		Ask:        ask,
	}, true
}

func (state *DepthSymbol) ApplyFeedback(
	predictedReturn float64,
	actualReturn float64,
) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	_, err := state.forecast.Next(0, predictedReturn, actualReturn)

	return err
}
