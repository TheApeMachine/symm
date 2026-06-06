package depthflow

import (
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	symmarket "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
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
	mu                     sync.RWMutex
	symbol                 string
	book                   market.Book
	bookReady              bool
	bookDiverged           bool
	bookDepth              int
	spoofWeightedThreshold float64
	spoofLevel1Reject      float64
	last                   float64
	bid                    float64
	ask                    float64
	buyPressure            float64
	pressure               *adaptive.EMA
	score                  *numeric.Derived
	tracked                *types.Category
}

func NewDepthSymbol(symbol string) (*DepthSymbol, error) {
	depth, err := symmarket.RequiredBookDepthLevels()

	if err != nil {
		return nil, err
	}

	return &DepthSymbol{
		symbol:                 symbol,
		bookDepth:              depth,
		spoofWeightedThreshold: viper.GetFloat64("signals.spoof_weighted_threshold"),
		spoofLevel1Reject:      viper.GetFloat64("signals.spoof_level1_reject"),
		pressure:               adaptive.NewEMA(0),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		tracked: types.NewCategory(types.CategoryTypeNone),
	}, nil
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

func (state *DepthSymbol) quoteLastLocked() float64 {
	if state.last > 0 {
		return state.last
	}

	if state.bid > 0 && state.ask > 0 {
		mid := (state.bid + state.ask) / 2

		if mid > 0 {
			return mid
		}
	}

	bids := state.book.Bids
	asks := state.book.Asks

	if len(bids) == 0 || len(asks) == 0 {
		return 0
	}

	mid := (bids[0].Price + asks[0].Price) / 2

	if mid <= 0 {
		return 0
	}

	return mid
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
