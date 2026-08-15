package types

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
)

/*
QuoteHistory retains bounded causal quote context for tape signals. Generate
fans the same instance across symbol workers, so the window map is serialized.
*/
type QuoteHistory struct {
	mutex    sync.RWMutex
	capacity int
	windows  map[string]*quoteWindow
}

type quoteWindow struct {
	values []kraken.TickerData
	next   int
	count  int
}

/*
NewQuoteHistory creates fixed storage for each symbol observed by one owner.
*/
func NewQuoteHistory(capacity int) *QuoteHistory {
	if capacity <= 0 {
		panic("quote history: positive capacity required")
	}

	return &QuoteHistory{
		capacity: capacity,
		windows:  make(map[string]*quoteWindow),
	}
}

/*
Observe retains a valid two-sided quote without imposing arrival-time order.
*/
func (history *QuoteHistory) Observe(ticker kraken.TickerData) bool {
	if history == nil || ticker.Symbol == "" || ticker.Timestamp.IsZero() ||
		ticker.Bid == nil || ticker.Ask == nil || ticker.Bid.Sign() <= 0 ||
		ticker.Ask.Cmp(ticker.Bid) <= 0 {
		return false
	}

	history.mutex.Lock()
	defer history.mutex.Unlock()

	window := history.windows[ticker.Symbol]

	if window == nil {
		window = &quoteWindow{values: make([]kraken.TickerData, history.capacity)}
		history.windows[ticker.Symbol] = window
	}

	for index := range window.count {
		if window.values[index].Timestamp.Equal(ticker.Timestamp) {
			window.values[index] = ticker
			return true
		}
	}

	window.values[window.next] = ticker
	window.next = (window.next + 1) % history.capacity

	if window.count < history.capacity {
		window.count++
	}

	return true
}

/*
At returns the newest retained quote observed no later than eventTime.
*/
func (history *QuoteHistory) At(
	symbol string,
	eventTime time.Time,
) (kraken.TickerData, bool) {
	if history == nil || symbol == "" || eventTime.IsZero() {
		return kraken.TickerData{}, false
	}

	history.mutex.RLock()
	defer history.mutex.RUnlock()

	window := history.windows[symbol]

	if window == nil {
		return kraken.TickerData{}, false
	}

	quote := kraken.TickerData{}

	for index := range window.count {
		candidate := window.values[index]

		if candidate.Timestamp.After(eventTime) {
			continue
		}

		if quote.Timestamp.IsZero() || candidate.Timestamp.After(quote.Timestamp) {
			quote = candidate
		}
	}

	return quote, !quote.Timestamp.IsZero()
}
