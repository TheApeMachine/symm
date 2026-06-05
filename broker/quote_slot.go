package broker

import (
	"sync"
	"sync/atomic"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

type quoteSlot struct {
	mu     sync.Mutex
	quote  atomic.Pointer[Quote]
	book   atomic.Pointer[market.Book]
	prices []float64
}

func (cache *QuoteCache) slotFor(symbol string) *quoteSlot {
	slot, _ := cache.slots.LoadOrStore(symbol, &quoteSlot{})

	return slot.(*quoteSlot)
}

// quoteValue returns the stored Quote with Book replaced by the latest book
// atomic. Callers must hold slot.mu for a consistent Quote+Book pair; without
// the lock, Book from storeBook may be fresher than the other Quote fields
// loaded from current — intentional for readers that want the newest book overlay.
func (slot *quoteSlot) quoteValue() (Quote, bool) {
	current := slot.quote.Load()

	if current == nil {
		return Quote{}, false
	}

	quote := *current

	if book, ok := slot.bookValue(); ok {
		quote.Book = book
	}

	return quote, true
}

func (slot *quoteSlot) bookValue() (market.Book, bool) {
	current := slot.book.Load()

	if current == nil {
		return market.Book{}, false
	}

	return *current, true
}

func (slot *quoteSlot) storeQuote(quote Quote) {
	next := quote
	slot.quote.Store(&next)
}

func (slot *quoteSlot) storeBook(book market.Book) {
	next := book
	slot.book.Store(&next)
}

func (slot *quoteSlot) observeVolatility(price float64) float64 {
	if price <= 0 {
		return perspectives.DistinctPriceVolatility(slot.prices)
	}

	if len(slot.prices) == 0 || slot.prices[len(slot.prices)-1] != price {
		slot.prices = append(slot.prices, price)
	}

	window := quoteVolatilityWindow()

	if len(slot.prices) > window {
		slot.prices = append(slot.prices[:0], slot.prices[len(slot.prices)-window:]...)
	}

	return perspectives.DistinctPriceVolatility(slot.prices)
}

func quoteVolatilityWindow() int {
	window := perspectives.RegimeWindow()

	if window > 1 {
		return window
	}

	return 2
}
