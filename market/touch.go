package market

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/rawbus"
)

/*
TouchSnapshot is the shared bid/ask touch observed for a symbol.
*/
type TouchSnapshot struct {
	Symbol     string
	Bid        float64
	Ask        float64
	Last       float64
	ObservedAt time.Time
}

func (touch TouchSnapshot) Midpoint() (float64, error) {
	if touch.Bid <= 0 || touch.Ask <= 0 {
		return 0, errors.New("market touch: bid and ask are required")
	}

	midpoint := (touch.Bid + touch.Ask) / 2

	if midpoint <= 0 {
		return 0, errors.New("market touch: midpoint is invalid")
	}

	return midpoint, nil
}

func (touch TouchSnapshot) Spread() float64 {
	if touch.Ask <= touch.Bid {
		return 0
	}

	return touch.Ask - touch.Bid
}

func (touch TouchSnapshot) Fresh(now time.Time, maxAge time.Duration) bool {
	if touch.Symbol == "" || touch.ObservedAt.IsZero() || maxAge <= 0 {
		return false
	}

	age := now.Sub(touch.ObservedAt)

	if age < 0 {
		return false
	}

	return age <= maxAge
}

/*
TouchRegistry owns the canonical touch state per symbol for signals and execution.
*/
type TouchRegistry struct {
	ctx         context.Context
	cancel      context.CancelFunc
	bus         *internal.Bus
	books       *sync.Map
	maxQuoteAge time.Duration
}

func NewTouchRegistry(
	ctx context.Context,
	pool *qpool.Q[any],
) (*TouchRegistry, error) {
	ctx, cancel := context.WithCancel(ctx)

	tradingConfig, tradingErr := config.LoadTradingConfig()

	if tradingErr != nil {
		cancel()

		return nil, errnie.Error(tradingErr)
	}

	if tradingConfig.MaxQuoteAge <= 0 {
		cancel()

		return nil, errors.New("market touch: max quote age must be positive")
	}

	bus := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{internal.ChannelRaw},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelRaw, "market:touch"),
		},
	)

	return &TouchRegistry{
		ctx:         ctx,
		cancel:      cancel,
		bus:         bus,
		books:       &sync.Map{},
		maxQuoteAge: tradingConfig.MaxQuoteAge,
	}, nil
}

func (registry *TouchRegistry) Tick() error {
	for {
		select {
		case <-registry.ctx.Done():
			return registry.ctx.Err()
		default:
		}

		message, err := registry.bus.Receive(internal.ChannelRaw)

		if internal.IsShutdown(err) {
			return err
		}

		if internal.ReportError(err) != nil || message == nil {
			continue
		}

		switch rawbus.TypeFrom(message.Type) {
		case rawbus.TypeTicker:
			tickers, ok := message.Value.(*krakenmarket.TickerUpdates)

			if !ok || tickers == nil {
				errnie.Error(errors.New("market touch: invalid tickers"))
				continue
			}

			for _, ticker := range *tickers {
				registry.applyTicker(ticker)
			}
		case rawbus.TypeBook:
			books, ok := message.Value.(*krakenmarket.BookUpdates)

			if !ok || books == nil {
				errnie.Error(errors.New("market touch: invalid books"))
				continue
			}

			for _, book := range *books {
				registry.applyBook(book)
			}
		case rawbus.TypeTrade:
			trades, ok := message.Value.(*krakenmarket.TradeUpdates)

			if !ok || trades == nil {
				errnie.Error(errors.New("market touch: invalid trades"))
				continue
			}

			for _, trade := range *trades {
				registry.applyTrade(trade)
			}
		}
	}
}

func (registry *TouchRegistry) Load(
	symbol string,
	now time.Time,
) (TouchSnapshot, bool) {
	if registry == nil || symbol == "" {
		return TouchSnapshot{}, false
	}

	rawBook, ok := registry.books.Load(symbol)

	if !ok {
		return TouchSnapshot{}, false
	}

	book, bookOK := rawBook.(*TouchBook)

	if !bookOK {
		registry.books.Delete(symbol)

		return TouchSnapshot{}, false
	}

	snapshot, snapshotOK := book.Snapshot(symbol)

	if !snapshotOK {
		return TouchSnapshot{}, false
	}

	if !snapshot.Fresh(now, registry.maxQuoteAge) {
		return TouchSnapshot{}, false
	}

	return snapshot, true
}

func (registry *TouchRegistry) MaxQuoteAge() time.Duration {
	if registry == nil {
		return 0
	}

	return registry.maxQuoteAge
}

func (registry *TouchRegistry) applyTicker(ticker *krakenmarket.TickerUpdate) {
	if ticker == nil || ticker.Symbol == "" {
		return
	}

	observedAt := time.Now().UTC()
	book := registry.symbolBook(ticker.Symbol)
	book.ApplyTicker(ticker, observedAt)
}

func (registry *TouchRegistry) applyBook(bookUpdate *krakenmarket.BookUpdate) {
	if bookUpdate == nil || bookUpdate.Symbol == "" {
		return
	}

	observedAt := time.Now().UTC()
	book := registry.symbolBook(bookUpdate.Symbol)
	book.ApplyBookUpdate(bookUpdate, observedAt)
}

func (registry *TouchRegistry) applyTrade(trade *krakenmarket.TradeUpdate) {
	if trade == nil || trade.Symbol == "" {
		return
	}

	observedAt := time.Now().UTC()
	book := registry.symbolBook(trade.Symbol)
	book.ApplyTrade(trade, observedAt)
}

func (registry *TouchRegistry) symbolBook(symbol string) *TouchBook {
	raw, ok := registry.books.Load(symbol)

	if ok {
		book, bookOK := raw.(*TouchBook)

		if bookOK {
			return book
		}
	}

	book := NewTouchBook()
	actual, _ := registry.books.LoadOrStore(symbol, book)

	loaded, loadOK := actual.(*TouchBook)

	if loadOK {
		return loaded
	}

	return book
}

func (registry *TouchRegistry) Close() error {
	registry.cancel()

	return registry.bus.Close()
}

/*
SeedTouch installs a touch snapshot for tests and integration harnesses.
*/
func (registry *TouchRegistry) SeedTouch(snapshot TouchSnapshot) {
	if registry == nil || snapshot.Symbol == "" || snapshot.ObservedAt.IsZero() {
		return
	}

	if snapshot.Bid <= 0 || snapshot.Ask <= snapshot.Bid {
		return
	}

	book := registry.symbolBook(snapshot.Symbol)
	book.ApplyTicker(&krakenmarket.TickerUpdate{
		Symbol: snapshot.Symbol,
		Bid:    snapshot.Bid,
		Ask:    snapshot.Ask,
		Last:   snapshot.Last,
		BidQty: 1,
		AskQty: 1,
	}, snapshot.ObservedAt)
}
