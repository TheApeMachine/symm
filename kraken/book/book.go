package book

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/market"
)

const defaultBookDepth = 10

type depthSide struct {
	levels []market.BookLevel
}

type depthState struct {
	bids   depthSide
	asks   depthSide
	bidOK  bool
	askOK  bool
	update time.Time
}

type bookPublish func(
	symbol string,
	spreadBPS, imbalance, density, depthSlope float64,
	updatedAt time.Time,
)

/*
Book watches Kraken v2 order book updates and exposes per-symbol depth and imbalance.
*/
type Book struct {
	ctx              context.Context
	mu               sync.RWMutex
	bySymbol         map[string]float64
	spreadBPS        map[string]float64
	density          map[string]float64
	depthSlope       map[string]float64
	ready            map[string]bool
	updatedAt        map[string]time.Time
	depths           map[string]depthState
	activityListener func(symbol string)
	publish          bookPublish
}

/*
New subscribes on the shared public websocket and registers the book handler.
*/
func New(
	parent context.Context,
	publicClient *client.PublicClient,
	symbols []string,
	publish bookPublish,
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
		ctx:        parent,
		bySymbol:   make(map[string]float64, len(symbols)),
		spreadBPS:  make(map[string]float64, len(symbols)),
		density:    make(map[string]float64, len(symbols)),
		depthSlope: make(map[string]float64, len(symbols)),
		ready:      make(map[string]bool, len(symbols)),
		updatedAt:  make(map[string]time.Time, len(symbols)),
		depths:     make(map[string]depthState, len(symbols)),
		publish:    publish,
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
DepthSlope returns cumulative volume per price step across stored book levels.
*/
func (book *Book) DepthSlope(symbol string) (float64, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	if !book.ready[symbol] {
		return 0, false
	}

	return book.depthSlope[symbol], true
}

/*
Depth returns up to depthLevels bid and ask levels for one symbol.
*/
func (book *Book) Depth(
	symbol string,
	depthLevels int,
) (bids, asks []market.BookLevel, ok bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	state, exists := book.depths[symbol]

	if !exists || !book.ready[symbol] || !state.bidOK || !state.askOK {
		return nil, nil, false
	}

	if depthLevels <= 0 {
		depthLevels = config.System.BookDepthLevels
	}

	bids = copyLevels(state.bids.levels, depthLevels)
	asks = copyLevels(state.asks.levels, depthLevels)

	return bids, asks, len(bids) > 0 && len(asks) > 0
}

/*
UpdatedAt returns when the merged book last changed for one symbol.
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
	delta, err := market.ParseBookLevelsDelta(payload)
	if err != nil {
		return nil
	}

	book.applyLevelsDelta(delta)

	return nil
}

func (book *Book) applyLevelsDelta(delta market.BookLevelsDelta) {
	now := time.Now()

	book.mu.Lock()
	state := book.depths[delta.Symbol]

	if delta.BidOK {
		state.bids.levels = mergeSideLevels(state.bids.levels, delta.Bids, true)
		state.bidOK = len(state.bids.levels) > 0
	}

	if delta.AskOK {
		state.asks.levels = mergeSideLevels(state.asks.levels, delta.Asks, false)
		state.askOK = len(state.asks.levels) > 0
	}

	if !state.bidOK || !state.askOK {
		book.depths[delta.Symbol] = state
		book.mu.Unlock()

		return
	}

	state.update = now

	bestBid := state.bids.levels[0]
	bestAsk := state.asks.levels[0]

	if delta.BidOK && len(delta.Bids) > 0 {
		bestBid = delta.Bids[0]
	}

	if delta.AskOK && len(delta.Asks) > 0 {
		bestAsk = delta.Asks[0]
	}

	top := market.BookTop{
		Symbol:  delta.Symbol,
		BestBid: bestBid,
		BestAsk: bestAsk,
	}

	listener := book.activityListener
	spread := spreadBPS(top)
	imbalance := topImbalance(top)
	density := topDensity(top)
	depthSlope := combinedDepthSlope(state)
	book.depths[delta.Symbol] = state
	book.bySymbol[delta.Symbol] = imbalance
	book.spreadBPS[delta.Symbol] = spread
	book.density[delta.Symbol] = density
	book.depthSlope[delta.Symbol] = depthSlope
	book.ready[delta.Symbol] = true
	book.updatedAt[delta.Symbol] = now
	publish := book.publish
	book.mu.Unlock()

	if publish != nil {
		publish(delta.Symbol, spread, imbalance, density, depthSlope, now)
	}

	if listener != nil {
		listener(delta.Symbol)
	}
}

func mergeSideLevels(
	existing, delta []market.BookLevel,
	bidSide bool,
) []market.BookLevel {
	byPrice := make(map[float64]float64, len(existing)+len(delta))

	for _, level := range existing {
		if level.Volume > 0 {
			byPrice[level.Price] = level.Volume
		}
	}

	for _, level := range delta {
		if level.Volume <= 0 {
			delete(byPrice, level.Price)
			continue
		}

		byPrice[level.Price] = level.Volume
	}

	merged := make([]market.BookLevel, 0, len(byPrice))

	for price, volume := range byPrice {
		merged = append(merged, market.BookLevel{Price: price, Volume: volume})
	}

	sort.Slice(merged, func(left, right int) bool {
		if bidSide {
			return merged[left].Price > merged[right].Price
		}

		return merged[left].Price < merged[right].Price
	})

	if len(merged) > defaultBookDepth {
		merged = merged[:defaultBookDepth]
	}

	return merged
}

func copyLevels(levels []market.BookLevel, depthLevels int) []market.BookLevel {
	if depthLevels <= 0 || len(levels) == 0 {
		return nil
	}

	if depthLevels > len(levels) {
		depthLevels = len(levels)
	}

	copied := make([]market.BookLevel, depthLevels)
	copy(copied, levels[:depthLevels])

	return copied
}

func combinedDepthSlope(state depthState) float64 {
	bidSlope := market.DepthSlope(state.bids.levels)
	askSlope := market.DepthSlope(state.asks.levels)

	return (bidSlope + askSlope) / 2
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
