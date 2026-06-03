package broker

import (
	"context"
	"encoding/json"
	"sync"
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
	Symbol    string
	Bid       float64
	Ask       float64
	Last      float64
	UpdatedAt time.Time
	Book      market.Book
}

type quoteListener func(symbol string, quote Quote)

type tradeListener func(symbol string, trade market.TradeUpdate)

/*
QuoteCache ingests public ticker and book frames from the raw bus.
*/
type QuoteCache struct {
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex
	quotes         map[string]Quote
	books          map[string]market.Book
	listeners      []quoteListener
	tradeListeners []tradeListener
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
	go sharedQuotes.run(pool)

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

	return &QuoteCache{
		ctx:    ctx,
		cancel: cancel,
		quotes: make(map[string]Quote),
		books:  make(map[string]market.Book),
	}
}

func (cache *QuoteCache) Subscribe(listener quoteListener) {
	if listener == nil {
		return
	}

	cache.mu.Lock()
	cache.listeners = append(cache.listeners, listener)
	cache.mu.Unlock()
}

func (cache *QuoteCache) SubscribeTrades(listener tradeListener) {
	if listener == nil {
		return
	}

	cache.mu.Lock()
	cache.tradeListeners = append(cache.tradeListeners, listener)
	cache.mu.Unlock()
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
	cache.mu.Lock()
	defer cache.mu.Unlock()

	quote := cache.quotes[row.Symbol]
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
	quote.Book = cache.books[row.Symbol]
	cache.quotes[row.Symbol] = quote

	cache.notifyLocked(row.Symbol, quote)
}

func (cache *QuoteCache) updateBook(row market.Book) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	book := cache.books[row.Symbol]
	book.Fold(row, bookDepthLevels)
	cache.books[row.Symbol] = book

	quote := cache.quotes[row.Symbol]
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
	cache.quotes[row.Symbol] = quote

	cache.notifyLocked(row.Symbol, quote)
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
	listeners := append([]tradeListener(nil), cache.tradeListeners...)

	for _, listener := range listeners {
		listener(trade.Symbol, trade)
	}
}

func (cache *QuoteCache) notifyLocked(symbol string, quote Quote) {
	listeners := append([]quoteListener(nil), cache.listeners...)

	for _, listener := range listeners {
		listener(symbol, quote)
	}
}

/*
Snapshot returns the latest quote for one symbol.
*/
func (cache *QuoteCache) Snapshot(symbol string) (Quote, bool) {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	quote, ok := cache.quotes[symbol]

	return quote, ok
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

	cache.mu.Lock()
	cache.quotes[quote.Symbol] = quote
	cache.mu.Unlock()
}
