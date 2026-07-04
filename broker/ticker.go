package broker

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
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

type tickerFrame struct {
	Data []map[string]any `json:"data"`
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
func (ticker *Ticker) Update(artifact *datura.Artifact) error {
	if ticker == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "ticker book is nil", nil))
	}

	if artifact == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "ticker artifact is nil", nil))
	}

	var frame tickerFrame
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &frame); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: decode ticker frame",
			err,
		))
	}

	updates := ticker.quotesFromFrame(frame, artifact.Timestamp())
	if len(updates) == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: ticker frame contained no usable quotes",
			nil,
		))
	}

	for {
		oldSnapshot := ticker.snapshot.Load()
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

func (ticker *Ticker) quotesFromFrame(
	frame tickerFrame,
	timestamp int64,
) map[string]MarketQuote {
	updates := map[string]MarketQuote{}
	updatedAt := time.Now().UTC()
	if timestamp > 0 {
		updatedAt = time.Unix(0, timestamp).UTC()
	}

	for _, row := range frame.Data {
		symbol := strings.TrimSpace(stringValue(row["symbol"]))
		if symbol == "" {
			continue
		}

		last, lastOK := numericValue(row["last"])
		bid, bidOK := numericValue(row["bid"])
		ask, askOK := numericValue(row["ask"])
		if !lastOK {
			last, _ = numericValue(row["price"])
		}

		if last <= 0 && (!bidOK || !askOK || bid <= 0 || ask <= 0) {
			continue
		}

		updates[symbol] = MarketQuote{
			Symbol:    symbol,
			Bid:       bid,
			Ask:       ask,
			Last:      last,
			UpdatedAt: updatedAt,
		}
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
