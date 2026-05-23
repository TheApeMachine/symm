package ticker

import (
	"context"
	"fmt"
	"sync"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/market"
)

type quoteRow struct {
	last      float64
	bid       float64
	ask       float64
	changePct float64
	volume    float64
	timestamp string
}

type quoteListener func(symbol string, last, bid, ask, changePct float64, timestamp string)

/*
Ticker watches Kraken v2 ticker updates for price and 24h volume.
*/
type Ticker struct {
	ctx            context.Context
	mu             sync.RWMutex
	quotes         map[string]quoteRow
	ready          map[string]bool
	quoteListeners []quoteListener
}

/*
New subscribes on the shared public websocket and registers the ticker handler.
*/
func New(
	parent context.Context,
	publicClient *client.PublicClient,
	symbols []string,
) (*Ticker, error) {
	if len(symbols) == 0 {
		return nil, fmt.Errorf("ticker observer requires at least one symbol")
	}

	if publicClient == nil {
		return nil, fmt.Errorf("public websocket client is nil")
	}

	if err := client.SubscribeSymbolsBatched(
		publicClient,
		symbols,
		config.System.SubscribeBatch,
		func(chunk []string) any {
			return market.SubscribeParams{}.Ticker(chunk)
		},
	); err != nil {
		return nil, fmt.Errorf("subscribe ticker channel: %w", err)
	}

	ticker := &Ticker{
		ctx:    parent,
		quotes: make(map[string]quoteRow, len(symbols)),
		ready:  make(map[string]bool, len(symbols)),
	}

	publicClient.OnFrame(ticker.handleFrame)

	return ticker, nil
}

/*
OnQuote registers a listener invoked synchronously for each ticker row update.
*/
func (ticker *Ticker) OnQuote(listener quoteListener) {
	ticker.mu.Lock()
	defer ticker.mu.Unlock()

	ticker.quoteListeners = append(ticker.quoteListeners, listener)
}

/*
ReadyCount returns how many symbols have received at least one quote.
*/
func (ticker *Ticker) ReadyCount() int {
	ticker.mu.RLock()
	defer ticker.mu.RUnlock()

	count := 0

	for _, ok := range ticker.ready {
		if ok {
			count++
		}
	}

	return count
}

/*
Last returns the latest trade price for one symbol.
*/
func (ticker *Ticker) Last(symbol string) (float64, bool) {
	row, ok := ticker.quote(symbol)
	if !ok {
		return 0, false
	}

	return row.last, true
}

/*
Quote returns last, bid, ask, and 24h change percent for one symbol.
*/
func (ticker *Ticker) Quote(symbol string) (last, bid, ask, changePct float64, ok bool) {
	row, ready := ticker.quote(symbol)
	if !ready {
		return 0, 0, 0, 0, false
	}

	return row.last, row.bid, row.ask, row.changePct, true
}

/*
Timestamp returns the exchange timestamp string for one symbol quote.
*/
func (ticker *Ticker) Timestamp(symbol string) (string, bool) {
	row, ok := ticker.quote(symbol)
	if !ok {
		return "", false
	}

	return row.timestamp, true
}

/*
VolumeBase returns 24h base volume for one symbol.
*/
func (ticker *Ticker) VolumeBase(symbol string) (float64, bool) {
	ticker.mu.RLock()
	defer ticker.mu.RUnlock()

	if !ticker.ready[symbol] {
		return 0, false
	}

	return ticker.quotes[symbol].volume, true
}

func (ticker *Ticker) quote(symbol string) (quoteRow, bool) {
	ticker.mu.RLock()
	defer ticker.mu.RUnlock()

	if !ticker.ready[symbol] {
		return quoteRow{}, false
	}

	return ticker.quotes[symbol], true
}

func (ticker *Ticker) handleFrame(_ context.Context, payload []byte) error {
	rows, err := market.ParseTickerRows(payload)
	if err != nil {
		return nil
	}

	ticker.store(rows)

	return nil
}

func (ticker *Ticker) store(rows []market.TickerRow) {
	ticker.mu.Lock()
	listeners := append([]quoteListener(nil), ticker.quoteListeners...)

	for _, row := range rows {
		ticker.quotes[row.Symbol] = quoteRow{
			last:      row.Last,
			bid:       row.Bid,
			ask:       row.Ask,
			changePct: row.ChangePct,
			volume:    row.Volume,
			timestamp: row.Timestamp,
		}
		ticker.ready[row.Symbol] = true
	}

	ticker.mu.Unlock()

	for _, row := range rows {
		for _, listener := range listeners {
			listener(row.Symbol, row.Last, row.Bid, row.Ask, row.ChangePct, row.Timestamp)
		}
	}
}
