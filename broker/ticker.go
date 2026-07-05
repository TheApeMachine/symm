package broker

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Ticker is the broker's immutable quote book.
It keeps the most recent bid, ask, and last price per symbol without locks.
*/
type Ticker struct {
	snapshot atomic.Pointer[quoteSnapshot]
}

type quoteSnapshot struct {
	quotes map[string]MarketQuote
}

/*
MarketQuote is the executable reference price surface for one symbol.
*/
type MarketQuote struct {
	Symbol    string
	Bid       float64
	Ask       float64
	Last      float64
	UpdatedAt time.Time
}

/*
NewTicker instantiates an empty quote book.
*/
func NewTicker() *Ticker {
	ticker := &Ticker{}
	ticker.snapshot.Store(&quoteSnapshot{quotes: map[string]MarketQuote{}})

	return ticker
}

/*
Update merges a ticker frame into the latest quote snapshot.
*/
func (ticker *Ticker) Update(frame map[string]any) error {
	if ticker == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "ticker book is nil", nil))
	}

	if len(frame) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "ticker frame is empty", nil))
	}

	rows, err := ticker.rows(frame)
	if err != nil {
		return err
	}

	observedAt, err := ticker.timestamp(frame)
	if err != nil {
		return err
	}

	for {
		oldSnapshot := ticker.snapshot.Load()
		updates := ticker.quotesFromFrame(rows, oldSnapshot, observedAt)
		if len(updates) == 0 {
			return nil
		}

		next := map[string]MarketQuote{}
		if oldSnapshot != nil {
			for symbol, quote := range oldSnapshot.quotes {
				next[symbol] = quote
			}
		}

		for symbol, quote := range updates {
			next[symbol] = quote
		}

		newSnapshot := &quoteSnapshot{quotes: next}
		if ticker.snapshot.CompareAndSwap(oldSnapshot, newSnapshot) {
			return nil
		}
	}
}

func (ticker *Ticker) rows(frame map[string]any) ([]map[string]any, error) {
	switch data := frame["data"].(type) {
	case []map[string]any:
		return data, nil
	case []any:
		rows := make([]map[string]any, 0, len(data))
		for _, item := range data {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					"broker: ticker data row object required",
					nil,
				))
			}

			rows = append(rows, row)
		}

		return rows, nil
	default:
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: ticker data rows required",
			nil,
		))
	}
}

func (ticker *Ticker) timestamp(frame map[string]any) (int64, error) {
	observed := strings.TrimSpace(stringValue(frame["timestamp"]))
	if observed == "" {
		return 0, nil
	}

	at, err := time.Parse(time.RFC3339Nano, observed)
	if err != nil {
		return 0, errnie.Error(errnie.Err(errnie.Validation, "broker: ticker timestamp", err))
	}

	return at.UnixNano(), nil
}

func (ticker *Ticker) quotesFromFrame(
	rows []map[string]any,
	snapshot *quoteSnapshot,
	timestamp int64,
) map[string]MarketQuote {
	updates := map[string]MarketQuote{}
	updatedAt := time.Now().UTC()
	if timestamp > 0 {
		updatedAt = time.Unix(0, timestamp).UTC()
	}

	for _, row := range rows {
		symbol := strings.TrimSpace(stringValue(row["symbol"]))
		if symbol == "" {
			continue
		}

		quote := MarketQuote{Symbol: symbol}
		if snapshot != nil {
			quote = snapshot.quotes[symbol]
			quote.Symbol = symbol
		}

		if last, ok := numericValue(row["last"]); ok && last > 0 {
			quote.Last = last
		}

		if price, ok := numericValue(row["price"]); ok && price > 0 {
			quote.Last = price
		}

		if bid, ok := numericValue(row["bid"]); ok && bid > 0 {
			quote.Bid = bid
		}

		if ask, ok := numericValue(row["ask"]); ok && ask > 0 {
			quote.Ask = ask
		}

		if quote.Last <= 0 && quote.Bid <= 0 && quote.Ask <= 0 {
			continue
		}

		quote.UpdatedAt = updatedAt
		updates[symbol] = quote
	}

	return updates
}

/*
Quote returns the latest quote for the symbol.
*/
func (ticker *Ticker) Quote(symbol string) (MarketQuote, bool) {
	if ticker == nil {
		return MarketQuote{}, false
	}

	snapshot := ticker.snapshot.Load()
	if snapshot == nil || snapshot.quotes == nil {
		return MarketQuote{}, false
	}

	quote, ok := snapshot.quotes[strings.TrimSpace(symbol)]
	return quote, ok
}

/*
Price returns an executable reference price for the requested side.
*/
func (quote MarketQuote) Price(side string) float64 {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		if quote.Ask > 0 {
			return quote.Ask
		}
	case "sell":
		if quote.Bid > 0 {
			return quote.Bid
		}
	}

	return quote.Last
}

/*
PassivePrice returns the resting limit reference for the requested side.
*/
func (quote MarketQuote) PassivePrice(side string) float64 {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		if quote.Bid > 0 {
			return quote.Bid
		}
	case "sell":
		if quote.Ask > 0 {
			return quote.Ask
		}
	}

	return quote.Price(side)
}
