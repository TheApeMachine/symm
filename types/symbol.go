package types

import (
	"sync/atomic"
	"time"
)

/*
Symbol is the pure per-symbol shared state of the thesis. It carries no
pipeline machinery: no queues, no consumers, no work scheduling. Market data
flows over the runtime workspace bus; this type only holds identity, the
ingest clock, and the dedupe cursors that keep one symbol's event stream
monotonic.
*/
type Symbol struct {
	ID     SymbolID `json:"id,omitempty"`
	Symbol string   `json:"symbol,omitempty"`
	Status Status   `json:"status,omitempty"`
	Tick   int64    `json:"tick,omitempty"`

	lastTickerNano       atomic.Int64 `json:"-"`
	lastTradeNano        atomic.Int64 `json:"-"`
	lastLevel3Nano       atomic.Int64 `json:"-"`
	lastFuturesTickerNano atomic.Int64 `json:"-"`
	lastFuturesTradeNano  atomic.Int64 `json:"-"`
	lastFuturesBookNano   atomic.Int64 `json:"-"`
}

/*
NewSymbol creates the empty shared state for one market symbol.
*/
func NewSymbol(name string) *Symbol {
	return &Symbol{Symbol: name, Status: READY}
}

/*
AcceptTicker reports whether a ticker is newer than the last accepted one,
advancing the dedupe cursor when it is. The ingest path publishes only
accepted events to the bus, so a symbol's stream stays monotonic.
*/
func (symbol *Symbol) AcceptTicker(timestamp time.Time) bool {
	return acceptNano(&symbol.lastTickerNano, timestamp)
}

func (symbol *Symbol) AcceptTrade(timestamp time.Time) bool {
	return acceptNano(&symbol.lastTradeNano, timestamp)
}

func (symbol *Symbol) AcceptLevel3(timestamp time.Time) bool {
	return acceptNano(&symbol.lastLevel3Nano, timestamp)
}

func (symbol *Symbol) AcceptFuturesTicker(timestamp time.Time) bool {
	return acceptNano(&symbol.lastFuturesTickerNano, timestamp)
}

func (symbol *Symbol) AcceptFuturesTrade(timestamp time.Time) bool {
	return acceptNano(&symbol.lastFuturesTradeNano, timestamp)
}

func (symbol *Symbol) AcceptFuturesBook(timestamp time.Time) bool {
	return acceptNano(&symbol.lastFuturesBookNano, timestamp)
}

func acceptNano(cursor *atomic.Int64, timestamp time.Time) bool {
	if timestamp.IsZero() {
		return true
	}

	nano := timestamp.UnixNano()
	last := cursor.Load()

	if nano < last {
		return false
	}

	for nano > last {
		if cursor.CompareAndSwap(last, nano) {
			return true
		}

		last = cursor.Load()

		if nano < last {
			return false
		}
	}

	return true
}
