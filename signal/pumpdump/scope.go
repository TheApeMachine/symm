package pumpdump

import (
	"time"

	"github.com/theapemachine/symm/signal"
)

/*
ScopeSnapshot holds scoped input facts assembled from trade and book rings.
*/
type ScopeSnapshot struct {
	Price     float64
	Volume    float64
	Spread    float64
	Move      float64
	Precursor float64
	Elapsed   float64
	Observed  time.Time
}

/*
TradeScopeSnapshot builds a scoped snapshot from the trade click clock.
*/
func TradeScopeSnapshot(tradeFeed *signal.Trade, scope string) ScopeSnapshot {
	window, ok := tradeFeed.Window(scope)

	if !ok {
		return ScopeSnapshot{}
	}

	move, precursor := signal.AnchorChange(window.Prices[0], window.Prices[len(window.Prices)-1])
	spread, spreadOK := signal.TouchSpread(window.Prices)

	if !spreadOK {
		return ScopeSnapshot{}
	}

	return ScopeSnapshot{
		Price:     window.Prices[len(window.Prices)-1],
		Volume:    window.Volume,
		Spread:    spread,
		Move:      move,
		Precursor: precursor,
		Observed:  window.Latest.Timestamp,
		Elapsed:   window.Elapsed,
	}
}

/*
BookScopeSnapshot builds a scoped snapshot from the book list ring.
*/
func BookScopeSnapshot(bookFeed *signal.Book, scope string) ScopeSnapshot {
	window, ok := bookFeed.Window(scope)

	if !ok {
		return ScopeSnapshot{}
	}

	move, precursor := signal.AnchorChange(window.Prices[0], window.Prices[len(window.Prices)-1])

	return ScopeSnapshot{
		Price:     window.Prices[len(window.Prices)-1],
		Spread:    window.Spreads[len(window.Spreads)-1],
		Move:      move,
		Precursor: precursor,
		Observed:  window.Latest.Timestamp,
	}
}
