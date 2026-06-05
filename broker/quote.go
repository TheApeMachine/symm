package broker

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

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
}

var (
	quoteCacheMu sync.Mutex
	sharedQuotes *QuoteCache
)

/*
EnsureQuoteCache returns the process-wide quote cache bound to a live context.
*/
func EnsureQuoteCache(ctx context.Context, pool *qpool.Q) *QuoteCache {
	quoteCacheMu.Lock()
	defer quoteCacheMu.Unlock()

	if sharedQuotes != nil && sharedQuotes.ctx.Err() == nil {
		return sharedQuotes
	}

	if sharedQuotes != nil {
		sharedQuotes.cancel()
	}

	sharedQuotes = NewQuoteCache(ctx, pool)
	sharedQuotes.Start(pool)

	return sharedQuotes
}

/*
ResetQuoteCacheForTest tears down the shared cache between isolated harness runs.
*/
func ResetQuoteCacheForTest() {
	quoteCacheMu.Lock()
	defer quoteCacheMu.Unlock()

	if sharedQuotes == nil {
		return
	}

	sharedQuotes.cancel()
	sharedQuotes = nil
}

func NewQuoteCache(ctx context.Context, _ *qpool.Q) *QuoteCache {
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
func (cache *QuoteCache) Start(pool *qpool.Q) {
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

func (cache *QuoteCache) run(pool *qpool.Q) {
	if pool == nil {
		return
	}

	group := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	subscriber := group.Subscribe("broker:quotes", 4096)

	if subscriber == nil {
		return
	}

	for {
		select {
		case <-cache.ctx.Done():
			return
		case message, ok := <-subscriber.Incoming:
			if !ok {
				return
			}

			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(map[string]any)

			if !ok {
				continue
			}

			channel, _ := envelope["channel"].(string)
			rawData, _ := envelope["data"].(json.RawMessage)
			frame := &public.SocketMessage{Channel: channel, Data: rawData}

			switch channel {
			case public.TickerChannel:
				cache.ingestTickers(frame)
			case public.BookChannel:
				cache.ingestBooks(frame)
			case public.TradesChannel:
				cache.ingestTrades(frame)
			}
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

		cache.notifyTradeLocked(row)
	}
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

	return quoteSlot.quoteValue()
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
