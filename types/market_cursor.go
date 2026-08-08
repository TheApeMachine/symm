package types

import (
	"sort"
	"time"

	"github.com/theapemachine/symm/kraken"
)

/*
marketInput identifies one raw Thesis stream without conflating its cursor with
the other stream consumed by the same signal.
*/
type marketInput string

const (
	marketTickers marketInput = "tickers"
	marketTrades  marketInput = "trades"
)

/*
marketCursorKey isolates committed and staged progress by signal, input, and
symbol so a faster market cannot consume another symbol's observations.
*/
type marketCursorKey struct {
	source  SourceType
	input   marketInput
	symbol  string
	pending bool
}

/*
marketCursor retains the latest incorporated exchange epoch and all distinct
trade IDs observed at that exact timestamp.
*/
type marketCursor struct {
	at       time.Time
	tradeIDs map[int64]struct{}
}

/*
MarketTickers returns ticker observations this signal source has not committed.
Reading stages a cursor; publishing a measurement commits it, so a signal that
refuses an incomplete input can retry it when the missing input arrives.
*/
func (thesis *Thesis) MarketTickers(source SourceType) []kraken.TickerData {
	tickers := make([]kraken.TickerData, 0)

	thesis.Tickers.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		rows, rowsOK := value.([]kraken.TickerData)

		if !symbolOK || !rowsOK {
			return true
		}

		rows = append([]kraken.TickerData(nil), rows...)
		sort.SliceStable(rows, func(left, right int) bool {
			return rows[left].Timestamp.Before(rows[right].Timestamp)
		})

		cursor := thesis.marketCursor(source, marketTickers, symbol)
		candidate := cursor

		for _, ticker := range rows {
			if !ticker.Timestamp.After(cursor.at) {
				continue
			}

			tickers = append(tickers, ticker)
			candidate.at = ticker.Timestamp
		}

		thesis.stageMarketCursor(source, marketTickers, symbol, cursor, candidate)
		return true
	})

	return tickers
}

/*
MarketTrades returns trade observations this signal source has not committed.
Trade IDs preserve distinct executions that share an exchange timestamp.
*/
func (thesis *Thesis) MarketTrades(source SourceType) []kraken.TradeData {
	trades := make([]kraken.TradeData, 0)

	thesis.Trades.Range(func(key, value any) bool {
		symbol, symbolOK := key.(string)
		rows, rowsOK := value.([]kraken.TradeData)

		if !symbolOK || !rowsOK {
			return true
		}

		rows = append([]kraken.TradeData(nil), rows...)
		sort.SliceStable(rows, func(left, right int) bool {
			if rows[left].Timestamp.Equal(rows[right].Timestamp) {
				return rows[left].TradeID < rows[right].TradeID
			}

			return rows[left].Timestamp.Before(rows[right].Timestamp)
		})

		cursor := thesis.marketCursor(source, marketTrades, symbol)
		candidate := cursor.clone()

		for _, trade := range rows {
			if candidate.observed(trade) {
				continue
			}

			trades = append(trades, trade)
			candidate.observe(trade)
		}

		thesis.stageMarketCursor(source, marketTrades, symbol, cursor, candidate)
		return true
	})

	return trades
}

func (thesis *Thesis) marketCursor(
	source SourceType,
	input marketInput,
	symbol string,
) marketCursor {
	stored, found := thesis.Measured.Load(marketCursorKey{
		source: source,
		input:  input,
		symbol: symbol,
	})

	if !found {
		return marketCursor{}
	}

	return stored.(marketCursor)
}

func (thesis *Thesis) stageMarketCursor(
	source SourceType,
	input marketInput,
	symbol string,
	committed marketCursor,
	candidate marketCursor,
) {
	if candidate.at.Equal(committed.at) &&
		len(candidate.tradeIDs) == len(committed.tradeIDs) {
		return
	}

	thesis.Measured.Store(marketCursorKey{
		source:  source,
		input:   input,
		symbol:  symbol,
		pending: true,
	}, candidate)
}

func (thesis *Thesis) commitMarketInputs(
	source SourceType,
	measurements []*Measurement,
) {
	symbols := make(map[string]struct{}, len(measurements))

	for _, measurement := range measurements {
		symbols[measurement.Symbol] = struct{}{}

		if measurement.Peer != "" {
			symbols[measurement.Peer] = struct{}{}
		}
	}

	thesis.Measured.Range(func(key, value any) bool {
		cursorKey, ok := key.(marketCursorKey)

		if !ok || !cursorKey.pending || cursorKey.source != source {
			return true
		}

		if _, measured := symbols[cursorKey.symbol]; !measured {
			return true
		}

		thesis.Measured.Store(marketCursorKey{
			source: cursorKey.source,
			input:  cursorKey.input,
			symbol: cursorKey.symbol,
		}, value)
		thesis.Measured.Delete(cursorKey)
		return true
	})
}

func (cursor marketCursor) clone() marketCursor {
	cloned := marketCursor{at: cursor.at}

	if len(cursor.tradeIDs) == 0 {
		return cloned
	}

	cloned.tradeIDs = make(map[int64]struct{}, len(cursor.tradeIDs))

	for tradeID := range cursor.tradeIDs {
		cloned.tradeIDs[tradeID] = struct{}{}
	}

	return cloned
}

func (cursor marketCursor) observed(trade kraken.TradeData) bool {
	if trade.Timestamp.Before(cursor.at) {
		return true
	}

	if trade.Timestamp.After(cursor.at) {
		return false
	}

	if trade.TradeID == 0 {
		return !cursor.at.IsZero()
	}

	_, found := cursor.tradeIDs[trade.TradeID]
	return found
}

func (cursor *marketCursor) observe(trade kraken.TradeData) {
	if trade.Timestamp.After(cursor.at) {
		cursor.at = trade.Timestamp
		cursor.tradeIDs = nil
	}

	if trade.TradeID == 0 {
		return
	}

	if cursor.tradeIDs == nil {
		cursor.tradeIDs = make(map[int64]struct{})
	}

	cursor.tradeIDs[trade.TradeID] = struct{}{}
}
