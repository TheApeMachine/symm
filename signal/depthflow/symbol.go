package depthflow

import (
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/toxicity"
)

/*
DepthSymbol owns the per-symbol book/flow state for one DepthFlow consumer and
classifies book shape onto the weight-of-the-book perspective. Confidence is
classification clarity — margin to the nearest category boundary; SNR is how
surprising that clarity is versus the symbol's own recent baseline.

Kraken sends a snapshot then checksum-verified deltas; the maintained market.Book
is folded locally so imbalance reads reflect the full book, not the last delta slice.
*/
type DepthSymbol struct {
	mu           sync.RWMutex
	symbol       string
	book         market.Book
	bookReady    bool
	bookDiverged bool
	bookDepth    int
	last         float64
	bid          float64
	ask          float64
	buyPressure  float64
	pressure     *adaptive.EMA
	score        *numeric.Derived
	tracked      *perspectives.Category
}

func NewDepthSymbol(symbol string) *DepthSymbol {
	depth := viper.GetViper().GetInt("market.book_depth_levels")

	if depth <= 0 {
		depth = 10
	}

	return &DepthSymbol{
		symbol:    symbol,
		bookDepth: depth,
		pressure:  adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		tracked: perspectives.NewCategory(perspectives.CategoryTypeNone),
	}
}

/*
ApplyBook folds one Kraken book frame into the maintained local book and verifies
the exchange checksum.
*/
func (state *DepthSymbol) ApplyBook(update market.Book) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.applyBookLocked(update)
}

func (state *DepthSymbol) applyBookLocked(update market.Book) {
	if update.IsSnapshot() {
		state.bookReady = true
		state.bookDiverged = false
		state.book.Fold(update, state.bookDepth)

		state.verifyBookChecksumLocked(update.Checksum)

		return
	}

	if !state.bookReady {
		return
	}

	state.book.Fold(update, state.bookDepth)
	state.verifyBookChecksumLocked(update.Checksum)
}

func (state *DepthSymbol) verifyBookChecksumLocked(expected int64) {
	if expected == 0 {
		return
	}

	if state.book.ComputedChecksum() != expected {
		state.bookDiverged = true
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

	return state.bookReady
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

	if state.bookDiverged || !state.bookReady {
		return state.measureTradePressureLocked()
	}

	bids := state.book.Bids
	asks := state.book.Asks
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

	if len(bids) == 0 || len(asks) == 0 {
		return state.measureTradePressureLocked()
	}

	imbalance, ok := state.weightedImbalanceLocked(bids, asks, mid)
	level1, levelOK := state.level1ImbalanceLocked(bids, asks)

	if !ok || imbalance == 0 || !levelOK {
		return state.measureTradePressureLocked()
	}

	flatImbalance, flatOK := state.flatImbalanceLocked(bids, asks)
	spoofed := state.isSpoofSkewLocked(imbalance, level1)

	if flatOK {
		spoofed = spoofed || state.isSpoofSkewLocked(flatImbalance, level1)
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
			category, evidence := depthflowReading(
				reasonDepthImbalance, imbalance, flatImbalance, flatOK, 0,
			)

			confidence, err := state.tracked.Observe(category, evidence)

			if err != nil {
				errnie.Error(err)

				return perspectives.Measurement{}, false
			}

			return perspectives.Measurement{
				Symbol:     state.symbol,
				Source:     perspectives.SourceDepthFlow,
				Category:   category,
				Strength:   raw,
				Confidence: confidence,
			}, true
		}
	}

	raw := math.Abs(level1)
	category, evidence := depthflowReading(
		reasonDepthSkeptic, imbalance, flatImbalance, flatOK, 0,
	)

	confidence, err := state.tracked.Observe(category, evidence)

	if err != nil {
		errnie.Error(err)

		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Symbol:     state.symbol,
		Source:     perspectives.SourceDepthFlow,
		Category:   category,
		Strength:   raw,
		Confidence: confidence,
	}, true
}

func (state *DepthSymbol) measureTradePressureLocked() (perspectives.Measurement, bool) {
	flow := math.Abs(state.buyPressure)

	if flow <= 0 {
		flow = math.Abs(state.pressure.Value())
	}

	if flow <= 0 {
		return perspectives.Measurement{}, false
	}

	category, evidence := depthflowReading("trade_pressure", 0, 0, false, flow)

	confidence, err := state.tracked.Observe(category, evidence)

	if err != nil {
		errnie.Error(err)

		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Symbol:     state.symbol,
		Source:     perspectives.SourceDepthFlow,
		Category:   category,
		Strength:   flow,
		Confidence: confidence,
	}, true
}

func (state *DepthSymbol) level1ImbalanceLocked(bids, asks []market.BookLevel) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].Qty + asks[0].Qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].Qty - asks[0].Qty) / total, true
}

func (state *DepthSymbol) flatImbalanceLocked(bids, asks []market.BookLevel) (float64, bool) {
	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range bids {
		bidVolume += level.Qty
	}

	for _, level := range asks {
		askVolume += level.Qty
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return 0, false
	}

	return (bidVolume - askVolume) / total, true
}

func (state *DepthSymbol) weightedImbalanceLocked(
	bids, asks []market.BookLevel, mid float64,
) (float64, bool) {
	if mid <= 0 {
		return 0, false
	}

	lambda := viper.GetViper().GetFloat64("signals.book_depth_decay_lambda")
	weightedBid := 0.0
	weightedAsk := 0.0

	for _, level := range bids {
		if state.isToxicLevelLocked(level.Price) {
			continue
		}

		weight := math.Exp(-lambda * math.Abs(level.Price-mid) / mid)
		weightedBid += level.Qty * weight
	}

	for _, level := range asks {
		if state.isToxicLevelLocked(level.Price) {
			continue
		}

		weight := math.Exp(-lambda * math.Abs(level.Price-mid) / mid)
		weightedAsk += level.Qty * weight
	}

	total := weightedBid + weightedAsk

	if total <= 0 {
		return 0, false
	}

	return (weightedBid - weightedAsk) / total, true
}

func (state *DepthSymbol) isToxicLevelLocked(price float64) bool {
	return toxicity.IsToxic(state.symbol, price, time.Now())
}

func (state *DepthSymbol) isSpoofSkewLocked(weighted, level1 float64) bool {
	weightedThreshold := viper.GetViper().GetFloat64("signals.spoof_weighted_threshold")
	level1Reject := viper.GetViper().GetFloat64("signals.spoof_level1_reject")

	if math.Abs(weighted) < weightedThreshold {
		return false
	}

	if weighted > 0 && level1 < level1Reject {
		return true
	}

	if weighted < 0 && level1 > -level1Reject {
		return true
	}

	return false
}
