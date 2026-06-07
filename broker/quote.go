package broker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	signalpool "github.com/theapemachine/symm/signal"
)

const bookDepthLevels = 10

/*
Quote is the cached top-of-book and L2 depth used for preflight and fill simulation.
*/
type Quote struct {
	Symbol     string
	Bid        float64
	Ask        float64
	Last       float64
	UpdatedAt  time.Time
	Book       market.Book
	Volatility float64
}

type quoteListener func(symbol string, quote Quote)

type tradeListener func(symbol string, trade market.TradeUpdate)

/*
QuoteCache ingests public ticker and book frames from the raw bus.
*/
type QuoteCache struct {
	ctx            context.Context
	cancel         context.CancelFunc
	slots          sync.Map
	listenerMu     sync.Mutex
	listeners      atomic.Pointer[[]quoteListener]
	tradeMu        sync.Mutex
	tradeListeners atomic.Pointer[[]tradeListener]
	started        atomic.Bool
	framesSeen     atomic.Uint64 // raw bus frames observed by the cache consumer
	quotesIngested atomic.Uint64 // ticker/book/trade frames folded into quote slots
}

/*
IngestStats reports how many raw frames the cache consumer has observed and how
many it folded into quote state. A watchdog uses the pair to distinguish "market
is quiet" (both flat) from "cache disconnected from the bus" (frames flow
elsewhere, both flat here) — the failure mode of the 2026-06-07 incident.
*/
func (cache *QuoteCache) IngestStats() (frames uint64, ingested uint64) {
	if cache == nil {
		return 0, 0
	}

	return cache.framesSeen.Load(), cache.quotesIngested.Load()
}

/*
EnsureQuoteCache constructs a fresh quote cache. Prefer runtime.Runtime at the
process root so live components share one instance via dependency injection.
*/
func EnsureQuoteCache(ctx context.Context, pool *qpool.Q[any]) *QuoteCache {
	cache := NewQuoteCache(ctx, pool)
	cache.Start(pool)

	return cache
}

func NewQuoteCache(ctx context.Context, _ *qpool.Q[any]) *QuoteCache {
	ctx, cancel := context.WithCancel(ctx)
	cache := &QuoteCache{
		ctx:    ctx,
		cancel: cancel,
	}

	listeners := make([]quoteListener, 0)
	tradeListeners := make([]tradeListener, 0)
	cache.listeners.Store(&listeners)
	cache.tradeListeners.Store(&tradeListeners)

	return cache
}

/*
Start begins ingesting raw Kraken quote frames into the cache.
*/
func (cache *QuoteCache) Start(pool *qpool.Q[any]) {
	if cache == nil || !cache.started.CompareAndSwap(false, true) {
		return
	}

	go cache.run(pool)
}

func (cache *QuoteCache) Subscribe(listener quoteListener) {
	if listener == nil {
		return
	}

	cache.listenerMu.Lock()
	defer cache.listenerMu.Unlock()

	current := cache.listeners.Load()
	next := append([]quoteListener(nil), (*current)...)
	next = append(next, listener)
	cache.listeners.Store(&next)
}

func (cache *QuoteCache) SubscribeTrades(listener tradeListener) {
	if listener == nil {
		return
	}

	cache.tradeMu.Lock()
	defer cache.tradeMu.Unlock()

	current := cache.tradeListeners.Load()
	next := append([]tradeListener(nil), (*current)...)
	next = append(next, listener)
	cache.tradeListeners.Store(&next)
}

func (cache *QuoteCache) run(pool *qpool.Q[any]) {
	if pool == nil {
		errnie.Error(errors.New("broker/quote: nil pool — quote cache will never ingest"), "broker/quote")
		return
	}

	// The cache MUST attach to the pool's shared "raw" group. qpool.NewBroadcastGroup
	// constructs a detached group nothing publishes to (see 2026-06-07 incident:
	// commit e26ef63b starved this cache for a full run). Pool registry only.
	group := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)

	if group == nil {
		errnie.Error(errors.New("broker/quote: raw broadcast group unavailable — quote cache will never ingest"), "broker/quote")
		return
	}

	consumer := group.Subscribe("broker:quotes", 4096)

	if consumer == nil {
		errnie.Error(errors.New("broker/quote: raw subscription failed — quote cache will never ingest"), "broker/quote")
		return
	}

	for {
		message, err := consumer.Wait(cache.ctx)

		if err != nil {
			return
		}

		if message == nil || message.Value == nil {
			continue
		}

		envelope, ok := message.Value.(map[string]any)

		if !ok {
			continue
		}

		cache.framesSeen.Add(1)

		channel, _ := envelope["channel"].(string)
		rawData, _ := envelope["data"].(json.RawMessage)
		frame := &public.SocketMessage{Channel: channel, Data: rawData}

		switch channel {
		case public.TickerChannel:
			cache.quotesIngested.Add(1)
			cache.ingestTickers(frame)
		case public.BookChannel:
			cache.quotesIngested.Add(1)
			cache.ingestBooks(frame)
		case public.TradesChannel:
			cache.quotesIngested.Add(1)
			cache.ingestTrades(frame)
		}
	}
}

func (cache *QuoteCache) ingestTickers(envelope *public.SocketMessage) {
	for _, row := range signalpool.GetTickers(envelope) {
		if row.Symbol == "" {
			continue
		}

		cache.updateTicker(row)
	}
}

func (cache *QuoteCache) ingestBooks(envelope *public.SocketMessage) {
	for _, row := range signalpool.GetBooks(envelope) {
		if row.Symbol == "" {
			continue
		}

		cache.updateBook(row)
	}
}

func (cache *QuoteCache) updateTicker(row market.TickerUpdate) {
	slot := cache.slotFor(row.Symbol)

	slot.mu.Lock()
	quote, _ := slot.quoteValue()
	quote.Symbol = row.Symbol

	if row.Bid > 0 {
		quote.Bid = row.Bid
	}

	if row.Ask > 0 {
		quote.Ask = row.Ask
	}

	if row.Last > 0 {
		quote.Last = row.Last
	}

	quote.UpdatedAt = time.Now().UTC()

	if book, ok := slot.bookValue(); ok {
		quote.Book = book
	}

	quote.Volatility = slot.observeVolatility(quote.Last)
	slot.storeQuote(quote)
	slot.mu.Unlock()

	cache.notify(row.Symbol, quote)
}

func (cache *QuoteCache) updateBook(row market.Book) {
	slot := cache.slotFor(row.Symbol)

	slot.mu.Lock()
	book, _ := slot.bookValue()
	book.Fold(row, bookDepthLevels)
	slot.storeBook(book)

	quote, _ := slot.quoteValue()
	quote.Symbol = row.Symbol
	quote.Book = book

	if len(book.Bids) > 0 && book.Bids[0].Price > 0 {
		quote.Bid = book.Bids[0].Price
	}

	if len(book.Asks) > 0 && book.Asks[0].Price > 0 {
		quote.Ask = book.Asks[0].Price
	}

	if quote.Last <= 0 && quote.Bid > 0 && quote.Ask > 0 {
		quote.Last = (quote.Bid + quote.Ask) / 2
	}

	quote.UpdatedAt = time.Now().UTC()
	quote.Volatility = slot.observeVolatility(quote.Last)
	slot.storeQuote(quote)
	slot.mu.Unlock()

	cache.notify(row.Symbol, quote)
}

func (cache *QuoteCache) ingestTrades(envelope *public.SocketMessage) {
	for _, row := range signalpool.GetTrades(envelope) {
		if row.Symbol == "" {
			continue
		}

		cache.updateTrade(row)
		cache.notifyTradeLocked(row)
	}
}

func (cache *QuoteCache) updateTrade(row market.TradeUpdate) {
	if row.Symbol == "" || row.Price <= 0 {
		return
	}

	updatedAt := row.Timestamp.UTC()

	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	slot := cache.slotFor(row.Symbol)
	slot.mu.Lock()
	quote, _ := slot.quoteValue()
	quote.Symbol = row.Symbol
	quote.Last = row.Price
	quote.UpdatedAt = updatedAt

	if book, ok := slot.bookValue(); ok {
		quote.Book = book
	}

	quote.Volatility = slot.observeVolatility(quote.Last)
	slot.storeQuote(quote)
	slot.mu.Unlock()

	cache.notify(row.Symbol, quote)
}

func (cache *QuoteCache) notifyTradeLocked(trade market.TradeUpdate) {
	listeners := cache.tradeListeners.Load()

	for _, listener := range *listeners {
		listener(trade.Symbol, trade)
	}
}

func (cache *QuoteCache) notify(symbol string, quote Quote) {
	listeners := cache.listeners.Load()

	for _, listener := range *listeners {
		listener(symbol, quote)
	}
}

/*
Snapshot returns the latest quote for one symbol.
*/
func (cache *QuoteCache) Snapshot(symbol string) (Quote, bool) {
	if cache == nil {
		return Quote{}, false
	}

	slot, ok := cache.slots.Load(symbol)

	if !ok {
		return Quote{}, false
	}

	quoteSlot := slot.(*quoteSlot)
	quoteSlot.mu.Lock()
	defer quoteSlot.mu.Unlock()

	quote, present := quoteSlot.quoteValue()

	// Fail closed if the slot ever holds another symbol's quote. updateTicker /
	// updateBook stamp quote.Symbol with the key they wrote under, so a mismatch
	// means a cross-symbol write corrupted this slot. The money paths (equity
	// projection, fills) call Snapshot — they must price a lot with that lot's own
	// quote or none, never a foreign one.
	if present && quote.Symbol != symbol {
		return Quote{}, false
	}

	return quote, present
}

/*
HasCompleteQuote reports bid and ask are both present.
*/
func (cache *QuoteCache) HasCompleteQuote(symbol string) bool {
	quote, ok := cache.Snapshot(symbol)

	if !ok {
		return false
	}

	return quote.Bid > 0 && quote.Ask > 0
}

/*
InstallQuoteForTest seeds one symbol quote for unit tests.
*/
func (cache *QuoteCache) InstallQuoteForTest(quote Quote) {
	if quote.Symbol == "" {
		return
	}

	if quote.UpdatedAt.IsZero() {
		quote.UpdatedAt = time.Now().UTC()
	}

	slot := cache.slotFor(quote.Symbol)
	slot.mu.Lock()

	if quote.Book.Symbol == "" {
		quote.Book.Symbol = quote.Symbol
	}

	if quote.Book.Symbol == quote.Symbol {
		slot.storeBook(quote.Book)
	}

	observedVolatility := slot.observeVolatility(quote.Last)

	if quote.Volatility <= 0 {
		quote.Volatility = observedVolatility
	}

	slot.storeQuote(quote)
	slot.mu.Unlock()
}
