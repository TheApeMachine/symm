package trader

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
)

/*
quoteCache holds the latest top-of-book and depth needed for paper fills that
match live SlippageFill / PreflightGates behaviour.
*/
type quoteCache struct {
	mu     sync.RWMutex
	quotes map[string]broker.Quote
}

func newQuoteCache() *quoteCache {
	return &quoteCache{quotes: make(map[string]broker.Quote)}
}

func (cache *quoteCache) ingestTicker(row market.TickerUpdate) {
	if row.Symbol == "" || row.Last <= 0 {
		return
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	quote := cache.quotes[row.Symbol]
	quote.Last = row.Last
	quote.Bid = row.Bid
	quote.Ask = row.Ask

	if quote.Bid <= 0 {
		quote.Bid = row.Last
	}

	if quote.Ask <= 0 {
		quote.Ask = row.Last
	}

	stampIngested(&quote)

	cache.quotes[row.Symbol] = quote
}

func (cache *quoteCache) ingestBook(update market.BookUpdate) {
	if update.Symbol == "" {
		return
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	quote := cache.quotes[update.Symbol]
	quote.BidDepth = copyLevels(update.Bids, quote.BidDepth)
	quote.AskDepth = copyLevels(update.Asks, quote.AskDepth)

	if update.IsSnapshot() {
		if len(update.Bids) > 0 {
			quote.Bid = update.Bids[0].Price
		}

		if len(update.Asks) > 0 {
			quote.Ask = update.Asks[0].Price
		}
	}

	stampIngested(&quote)

	cache.quotes[update.Symbol] = quote
}

// stampIngested records local receive time for PreflightGates freshness. Exchange
// wire timestamps lag delivery and can be minutes old on illiquid symbols while
// the cached top-of-book is still the venue's current state.
func stampIngested(quote *broker.Quote) {
	quote.At = time.Now()
}

func (cache *quoteCache) snapshot(symbol string, fallbackLast float64) broker.Quote {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	quote, ok := cache.quotes[symbol]

	if !ok {
		return broker.Quote{}
	}

	if quote.Last <= 0 {
		quote.Last = fallbackLast
	}

	return quote
}

func (cache *quoteCache) readyCount() int {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	ready := 0

	for _, quote := range cache.quotes {
		if quote.Last > 0 {
			ready++
		}
	}

	return ready
}

func (cache *quoteCache) lastPrices() map[string]float64 {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	out := make(map[string]float64, len(cache.quotes))

	for symbol, quote := range cache.quotes {
		if quote.Last > 0 {
			out[symbol] = quote.Last
		}
	}

	return out
}

func (cache *quoteCache) spreadBPS(symbol string) float64 {
	quote := cache.snapshot(symbol, 0)

	if quote.Bid <= 0 || quote.Ask <= 0 || quote.Ask < quote.Bid {
		return 0
	}

	mid := (quote.Bid + quote.Ask) / 2

	if mid <= 0 {
		return 0
	}

	return (quote.Ask - quote.Bid) / mid * 10000
}

func copyLevels(
	incoming []market.BookLevel,
	previous []market.BookLevel,
) []market.BookLevel {
	if len(incoming) == 0 {
		return previous
	}

	out := make([]market.BookLevel, len(incoming))

	for index := range incoming {
		out[index] = incoming[index]
	}

	return out
}
