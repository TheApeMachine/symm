package types

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
AppendFuturesTicker routes a futures ticker only to the signal owners
selected by the streaming topology.
*/
func (symbol *Symbol) AppendFuturesTicker(ticker kraken.FuturesTickerData) {
	if !ticker.Timestamp.IsZero() {
		nano := ticker.Timestamp.UnixNano()
		last := symbol.lastFuturesTickerNano.Load()

		if nano < last {
			return
		}

		for nano > last {
			if symbol.lastFuturesTickerNano.CompareAndSwap(last, nano) {
				break
			}

			last = symbol.lastFuturesTickerNano.Load()

			if nano < last {
				return
			}
		}
	}

	symbol.futuresTickers.Push(ticker)
}

/*
AppendFuturesTrade routes a futures trade or liquidation execution to the
derivatives signal processing queue.
*/
func (symbol *Symbol) AppendFuturesTrade(trade kraken.FuturesTradeData) {
	if !trade.Timestamp.IsZero() {
		nano := trade.Timestamp.UnixNano()
		last := symbol.lastFuturesTradeNano.Load()

		if nano < last {
			return
		}

		for nano > last {
			if symbol.lastFuturesTradeNano.CompareAndSwap(last, nano) {
				break
			}

			last = symbol.lastFuturesTradeNano.Load()

			if nano < last {
				return
			}
		}
	}

	symbol.futuresTrades.Push(trade)
}

/*
AppendFuturesBook routes top-of-book depth updates to the derivatives
observation stage.
*/
func (symbol *Symbol) AppendFuturesBook(book kraken.FuturesBookData) {
	if !book.Timestamp.IsZero() {
		nano := book.Timestamp.UnixNano()
		last := symbol.lastFuturesBookNano.Load()

		if nano < last {
			return
		}

		for nano > last {
			if symbol.lastFuturesBookNano.CompareAndSwap(last, nano) {
				break
			}

			last = symbol.lastFuturesBookNano.Load()

			if nano < last {
				return
			}
		}
	}

	symbol.futuresBooks.Push(book)
}

func (symbol *Symbol) HasFuturesTickersFor(
	consumer *transport.Consumer[kraken.FuturesTickerData],
) bool {
	return symbol.futuresTickers.Length(consumer) > 0
}

func (symbol *Symbol) HasFuturesTradesFor(
	consumer *transport.Consumer[kraken.FuturesTradeData],
) bool {
	return symbol.futuresTrades.Length(consumer) > 0
}

func (symbol *Symbol) HasFuturesBooksFor(
	consumer *transport.Consumer[kraken.FuturesBookData],
) bool {
	return symbol.futuresBooks.Length(consumer) > 0
}

/*
MarketFuturesTickers drains this source's futures ticker updates in transport
order, up to an event-time cut taken when the drain starts.
*/
func (symbol *Symbol) MarketFuturesTickers(
	consumer *transport.Consumer[kraken.FuturesTickerData],
) iter.Seq[kraken.FuturesTickerData] {
	cut := time.Now().UTC()

	return symbol.futuresTickers.Drain(consumer, func(ticker kraken.FuturesTickerData) bool {
		if ticker.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

/*
MarketFuturesTrades drains this source's futures trades and liquidations in
transport order, up to an event-time cut taken when the drain starts.
*/
func (symbol *Symbol) MarketFuturesTrades(
	consumer *transport.Consumer[kraken.FuturesTradeData],
) iter.Seq[kraken.FuturesTradeData] {
	cut := time.Now().UTC()

	return symbol.futuresTrades.Drain(consumer, func(trade kraken.FuturesTradeData) bool {
		if trade.Timestamp.After(cut) {
			return false
		}

		return true
	})
}

/*
MarketFuturesBooks drains this source's futures order book updates in transport
order, up to an event-time cut taken when the drain starts.
*/
func (symbol *Symbol) MarketFuturesBooks(
	consumer *transport.Consumer[kraken.FuturesBookData],
) iter.Seq[kraken.FuturesBookData] {
	cut := time.Now().UTC()

	return symbol.futuresBooks.Drain(consumer, func(book kraken.FuturesBookData) bool {
		if book.Timestamp.After(cut) {
			return false
		}

		return true
	})
}
