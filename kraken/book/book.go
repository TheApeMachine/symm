package book

import (
	"context"
	"fmt"
	"sync"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/market"
)

const defaultBookDepth = 10

/*
Book watches Kraken v2 order book updates and exposes per-symbol top-of-book imbalance.
*/
type Book struct {
	ctx       context.Context
	mu        sync.RWMutex
	bySymbol  map[string]float64
	spreadBPS map[string]float64
	density   map[string]float64
	ready     map[string]bool
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

	params := market.SubscribeParams{}.Book(symbols, defaultBookDepth)

	if err := publicClient.SubscribeTo(params); err != nil {
		return nil, fmt.Errorf("subscribe book channel: %w", err)
	}

	book := &Book{
		ctx:       parent,
		bySymbol:  make(map[string]float64, len(symbols)),
		spreadBPS: make(map[string]float64, len(symbols)),
		density:   make(map[string]float64, len(symbols)),
		ready:     make(map[string]bool, len(symbols)),
	}

	publicClient.OnFrame(book.handleFrame)

	return book, nil
}

/*
Observe returns mean bid-side imbalance across ready symbols in [-1, 1].
*/
func (book *Book) Observe(_ context.Context) (engine.Observation, error) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	if len(book.ready) == 0 {
		return engine.Observation{}, fmt.Errorf("book observer not ready")
	}

	var sum float64
	var count int

	for symbol, ok := range book.ready {
		if !ok {
			continue
		}

		sum += book.bySymbol[symbol]
		count++
	}

	if count == 0 {
		return engine.Observation{}, fmt.Errorf("book observer not ready")
	}

	return engine.Observation{Confidence: sum / float64(count)}, nil
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

func (book *Book) handleFrame(_ context.Context, payload []byte) error {
	top, err := market.ParseTopBook(payload)
	if err != nil {
		return nil
	}

	book.storeImbalance(top)

	return nil
}

func (book *Book) storeImbalance(top market.BookTop) {
	imbalance := topImbalance(top)
	spread := spreadBPS(top)

	book.mu.Lock()
	defer book.mu.Unlock()

	book.bySymbol[top.Symbol] = imbalance
	book.spreadBPS[top.Symbol] = spread
	book.density[top.Symbol] = topDensity(top)
	book.ready[top.Symbol] = true
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
