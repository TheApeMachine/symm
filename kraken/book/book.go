package book

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/market"
)

const defaultBookDepth = 10

type topState struct {
	bid    market.BookLevel
	ask    market.BookLevel
	bidOK  bool
	askOK  bool
	update time.Time
}

/*
Book watches Kraken v2 order book updates and exposes per-symbol top-of-book imbalance.
*/
type Book struct {
	ctx              context.Context
	mu               sync.RWMutex
	bySymbol         map[string]float64
	spreadBPS        map[string]float64
	density          map[string]float64
	ready            map[string]bool
	updatedAt        map[string]time.Time
	tops             map[string]topState
	activityListener func(symbol string)
}

/*
New subscribes on the shared public websocket and registers the book handler.
*/
func New(
	parent context.Context,
	publicClient *client.PublicClient,
	symbols []string,
) (*Book, error) {
	if len(symbols) == 0 {
		return nil, fmt.Errorf("book observer requires at least one symbol")
	}

	if publicClient == nil {
		return nil, fmt.Errorf("public websocket client is nil")
	}

	if err := client.SubscribeSymbolsBatched(
		publicClient,
		symbols,
		config.System.SubscribeBatch,
		func(chunk []string) any {
			return market.SubscribeParams{}.Book(chunk, defaultBookDepth)
		},
	); err != nil {
		return nil, fmt.Errorf("subscribe book channel: %w", err)
	}

	book := &Book{
		ctx:       parent,
		bySymbol:  make(map[string]float64, len(symbols)),
		spreadBPS: make(map[string]float64, len(symbols)),
		density:   make(map[string]float64, len(symbols)),
		ready:     make(map[string]bool, len(symbols)),
		updatedAt: make(map[string]time.Time, len(symbols)),
		tops:      make(map[string]topState, len(symbols)),
	}

	publicClient.OnFrame(book.handleFrame)

	return book, nil
}

/*
SetActivityListener registers a callback for order-book updates.
*/
func (book *Book) SetActivityListener(listener func(symbol string)) {
	book.mu.Lock()
	defer book.mu.Unlock()

	book.activityListener = listener
}

/*
Imbalance returns top-of-book bid dominance for one symbol in [-1, 1].
*/
func (book *Book) Imbalance(symbol string) (float64, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	if !book.ready[symbol] {
		return 0, false
	}

	return book.bySymbol[symbol], true
}

/*
SpreadBPS returns the latest bid-ask spread in basis points for one symbol.
*/
func (book *Book) SpreadBPS(symbol string) (float64, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	if !book.ready[symbol] {
		return 0, false
	}

	return book.spreadBPS[symbol], true
}

/*
Density returns top-of-book bid plus ask volume for one symbol.
*/
func (book *Book) Density(symbol string) (float64, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	if !book.ready[symbol] {
		return 0, false
	}

	return book.density[symbol], true
}

/*
UpdatedAt returns when the merged top-of-book last changed for one symbol.
*/
func (book *Book) UpdatedAt(symbol string) (time.Time, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	updated, ok := book.updatedAt[symbol]

	if !ok || !book.ready[symbol] {
		return time.Time{}, false
	}

	return updated, true
}

func (book *Book) handleFrame(_ context.Context, payload []byte) error {
	delta, err := market.ParseBookTopDelta(payload)
	if err != nil {
		return nil
	}

	book.applyTopDelta(delta)

	return nil
}

func (book *Book) applyTopDelta(delta market.BookTopDelta) {
	now := time.Now()

	book.mu.Lock()
	state := book.tops[delta.Symbol]

	if delta.BidOK {
		state.bid = delta.BestBid
		state.bidOK = true
	}

	if delta.AskOK {
		state.ask = delta.BestAsk
		state.askOK = true
	}

	if !state.bidOK || !state.askOK {
		book.tops[delta.Symbol] = state
		book.mu.Unlock()

		return
	}

	state.update = now
	top := market.BookTop{
		Symbol:  delta.Symbol,
		BestBid: state.bid,
		BestAsk: state.ask,
	}

	listener := book.activityListener
	book.tops[delta.Symbol] = state
	book.bySymbol[delta.Symbol] = topImbalance(top)
	book.spreadBPS[delta.Symbol] = spreadBPS(top)
	book.density[delta.Symbol] = topDensity(top)
	book.ready[delta.Symbol] = true
	book.updatedAt[delta.Symbol] = now
	book.mu.Unlock()

	if listener != nil {
		listener(delta.Symbol)
	}
}

func topDensity(top market.BookTop) float64 {
	return top.BestBid.Volume + top.BestAsk.Volume
}

func spreadBPS(top market.BookTop) float64 {
	bid := top.BestBid.Price
	ask := top.BestAsk.Price

	if bid <= 0 || ask <= 0 || ask < bid {
		return 0
	}

	mid := (bid + ask) / 2

	return (ask - bid) / mid * 10000
}

func topImbalance(top market.BookTop) float64 {
	total := top.BestBid.Volume + top.BestAsk.Volume

	if total <= 0 {
		return 0
	}

	return (top.BestBid.Volume - top.BestAsk.Volume) / total
}
