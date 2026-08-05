package types

import (
	"cmp"
	"slices"
	"sync"

	"github.com/theapemachine/symm/kraken"
)

/*
eventBuffer owns one symbol's append-only observations for a decision cycle.
*/
type eventBuffer[Event any] struct {
	mu     sync.RWMutex
	events []Event
}

func (buffer *eventBuffer[Event]) Append(event Event) {
	buffer.mu.Lock()
	buffer.events = append(buffer.events, event)
	buffer.mu.Unlock()
}

func (buffer *eventBuffer[Event]) Snapshot() []Event {
	buffer.mu.RLock()
	defer buffer.mu.RUnlock()

	return slices.Clone(buffer.events)
}

func (buffer *eventBuffer[Event]) AppendSnapshot(events []Event) []Event {
	buffer.mu.RLock()
	defer buffer.mu.RUnlock()

	return append(events, buffer.events...)
}

/*
AppendTicker retains a ticker observation until the decision cycle resets.
*/
func (thesis *Thesis) AppendTicker(ticker kraken.TickerData) {
	thesis.marketMu.RLock()
	defer thesis.marketMu.RUnlock()

	found, ok := thesis.Tickers.Load(ticker.Symbol)

	if !ok {
		created := &eventBuffer[kraken.TickerData]{}
		found, _ = thesis.Tickers.LoadOrStore(ticker.Symbol, created)
	}

	buffer, ok := found.(*eventBuffer[kraken.TickerData])

	if !ok {
		panic("thesis: ticker symbol contains an unexpected value type")
	}

	buffer.Append(ticker)
}

/*
AppendTrade retains a public execution until the decision cycle resets.
*/
func (thesis *Thesis) AppendTrade(trade kraken.TradeData) {
	thesis.marketMu.RLock()
	defer thesis.marketMu.RUnlock()

	found, ok := thesis.Trades.Load(trade.Symbol)

	if !ok {
		created := &eventBuffer[kraken.TradeData]{}
		found, _ = thesis.Trades.LoadOrStore(trade.Symbol, created)
	}

	buffer, ok := found.(*eventBuffer[kraken.TradeData])

	if !ok {
		panic("thesis: trade symbol contains an unexpected value type")
	}

	buffer.Append(trade)
}

/*
Market returns time-ordered ticker and trade snapshots for the current cycle.
*/
func (thesis *Thesis) Market() ([]kraken.TickerData, []kraken.TradeData) {
	return thesis.MarketTickers(), thesis.MarketTrades()
}

/*
MarketTickers returns a detached, time-ordered snapshot of ticker history.
*/
func (thesis *Thesis) MarketTickers() []kraken.TickerData {
	thesis.marketMu.RLock()
	defer thesis.marketMu.RUnlock()

	tickers := make([]kraken.TickerData, 0)

	thesis.Tickers.Range(func(_, value any) bool {
		buffer, buffered := value.(*eventBuffer[kraken.TickerData])

		if buffered {
			tickers = buffer.AppendSnapshot(tickers)
			return true
		}

		ticker, stored := value.(kraken.TickerData)

		if stored {
			tickers = append(tickers, ticker)
		}

		return true
	})

	slices.SortFunc(tickers, func(left, right kraken.TickerData) int {
		if ordered := cmp.Compare(
			left.Timestamp.UnixNano(),
			right.Timestamp.UnixNano(),
		); ordered != 0 {
			return ordered
		}

		return cmp.Compare(left.Symbol, right.Symbol)
	})

	return tickers
}

/*
MarketTrades returns a detached, time-ordered snapshot of trade history.
*/
func (thesis *Thesis) MarketTrades() []kraken.TradeData {
	thesis.marketMu.RLock()
	defer thesis.marketMu.RUnlock()

	trades := make([]kraken.TradeData, 0)

	thesis.Trades.Range(func(_, value any) bool {
		buffer, buffered := value.(*eventBuffer[kraken.TradeData])

		if buffered {
			trades = buffer.AppendSnapshot(trades)
			return true
		}

		trade, stored := value.(kraken.TradeData)

		if stored {
			trades = append(trades, trade)
		}

		return true
	})

	slices.SortFunc(trades, func(left, right kraken.TradeData) int {
		if ordered := cmp.Compare(
			left.Timestamp.UnixNano(),
			right.Timestamp.UnixNano(),
		); ordered != 0 {
			return ordered
		}

		if ordered := cmp.Compare(left.Symbol, right.Symbol); ordered != 0 {
			return ordered
		}

		return cmp.Compare(left.TradeID, right.TradeID)
	})

	return trades
}

/*
MarketSymbols returns the symbols represented by this cycle's market history.
*/
func (thesis *Thesis) MarketSymbols() []string {
	thesis.marketMu.RLock()
	defer thesis.marketMu.RUnlock()

	symbols := make(map[string]struct{})

	thesis.Tickers.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if ok && symbol != "" {
			symbols[symbol] = struct{}{}
			return true
		}

		ticker, ok := value.(kraken.TickerData)

		if ok && ticker.Symbol != "" {
			symbols[ticker.Symbol] = struct{}{}
		}

		return true
	})

	thesis.Trades.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if ok && symbol != "" {
			symbols[symbol] = struct{}{}
			return true
		}

		trade, ok := value.(kraken.TradeData)

		if ok && trade.Symbol != "" {
			symbols[trade.Symbol] = struct{}{}
		}

		return true
	})

	ordered := make([]string, 0, len(symbols))

	for symbol := range symbols {
		ordered = append(ordered, symbol)
	}

	slices.Sort(ordered)

	return ordered
}

/*
LatestTicker returns the most recent ticker timestamp for one symbol.
*/
func (thesis *Thesis) LatestTicker(symbol string) (kraken.TickerData, bool) {
	thesis.marketMu.RLock()
	defer thesis.marketMu.RUnlock()

	var latest kraken.TickerData
	found, ok := thesis.Tickers.Load(symbol)

	if !ok {
		return latest, false
	}

	buffer, buffered := found.(*eventBuffer[kraken.TickerData])

	if !buffered {
		latest, ok = found.(kraken.TickerData)
		return latest, ok
	}

	for _, ticker := range buffer.Snapshot() {
		if latest.Timestamp.After(ticker.Timestamp) {
			continue
		}

		latest = ticker
	}

	return latest, !latest.Timestamp.IsZero()
}
