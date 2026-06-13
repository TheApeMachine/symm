package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

func TestSymbolBookQuoteSnapshot(t *testing.T) {
	Convey("Given a merged L2 book", t, func() {
		book := newSymbolBook()
		now := time.Now().UTC()

		book.applyBookUpdate(&market.BookUpdate{
			Type: "snapshot",
			Bids: []market.BookLevel{{Price: 99.9, Qty: 1}},
			Asks: []market.BookLevel{{Price: 100.1, Qty: 2}},
		})

		Convey("It should expose the touch as a quote snapshot", func() {
			snapshot, ok := book.quoteSnapshot("BTC/USD", now)

			So(ok, ShouldBeTrue)
			So(snapshot.Bid, ShouldEqual, 99.9)
			So(snapshot.Ask, ShouldEqual, 100.1)
		})

		Convey("It should refresh touch from bid-side deltas", func() {
			book.applyBookUpdate(&market.BookUpdate{
				Type: "update",
				Bids: []market.BookLevel{{Price: 100.0, Qty: 3}},
			})

			snapshot, ok := book.quoteSnapshot("BTC/USD", now)

			So(ok, ShouldBeTrue)
			So(snapshot.Bid, ShouldEqual, 100.0)
			So(snapshot.Ask, ShouldEqual, 100.1)
		})
	})
}

func TestDeskRefreshesQuoteFromBook(t *testing.T) {
	Convey("Given a desk with a stale ticker quote", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk := NewDesk(ctx, pool)

		defer func() { _ = desk.Close() }()

		staleAt := time.Now().UTC().Add(-2 * time.Minute)
		desk.persistQuote(QuoteSnapshot{
			Symbol:     "SOSO/USD",
			Bid:        1.0,
			Ask:        1.02,
			Last:       1.01,
			ObservedAt: staleAt,
		})

		desk.onBook(&market.BookUpdate{
			Type:   "snapshot",
			Symbol: "SOSO/USD",
			Bids:   []market.BookLevel{{Price: 1.05, Qty: 10}},
			Asks:   []market.BookLevel{{Price: 1.06, Qty: 12}},
		})

		Convey("It should accept a new entry against the fresh book touch", func() {
			desk.onAction(&logic.Action{
				Type:     logic.ActionMarket,
				Side:     trading.Buy,
				Symbol:   "SOSO/USD",
				Price:    1.055,
				Quantity: 10,
			})

			actionOpen := false
			desk.actions.Range(func(any, any) bool {
				actionOpen = true
				return false
			})

			So(actionOpen, ShouldBeTrue)
		})
	})
}

func TestDeskEvictsExpiredQuote(t *testing.T) {
	Convey("Given a desk holding an expired quote snapshot", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk := NewDesk(ctx, pool)

		defer func() { _ = desk.Close() }()

		desk.persistQuote(QuoteSnapshot{
			Symbol:     "SOSO/USD",
			Bid:        1.0,
			Ask:        1.02,
			Last:       1.01,
			ObservedAt: time.Now().UTC().Add(-2 * time.Minute),
		})

		desk.onAction(&logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "SOSO/USD",
			Price:    1.01,
			Quantity: 10,
		})

		Convey("It should drop the quote and reject with quote required", func() {
			_, stillStored := desk.quotes.Load("SOSO/USD")
			So(stillStored, ShouldBeFalse)

			actionOpen := false
			desk.actions.Range(func(any, any) bool {
				actionOpen = true
				return false
			})

			So(actionOpen, ShouldBeFalse)
		})
	})
}

func TestDeskRefreshesQuoteLastFromTrade(t *testing.T) {
	Convey("Given a desk with a fresh book quote", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk := NewDesk(ctx, pool)

		defer func() { _ = desk.Close() }()

		desk.onBook(&market.BookUpdate{
			Type:   "snapshot",
			Symbol: "TAO/USD",
			Bids:   []market.BookLevel{{Price: 400, Qty: 1}},
			Asks:   []market.BookLevel{{Price: 401, Qty: 1}},
		})

		desk.onTrade(&market.TradeUpdate{
			Symbol: "TAO/USD",
			Price:  400.5,
			Qty:    0.1,
		})

		rawQuote, ok := desk.quotes.Load("TAO/USD")

		So(ok, ShouldBeTrue)

		quote := rawQuote.(QuoteSnapshot)

		Convey("It should refresh last and observation time from the trade", func() {
			So(quote.Last, ShouldEqual, 400.5)
			So(time.Since(quote.ObservedAt), ShouldBeLessThan, time.Second)
		})
	})
}

func TestDeskRefreshesPositionMarkFromBook(t *testing.T) {
	Convey("Given an armed long position", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk := NewDesk(ctx, pool)
		subscriber := internal.NewBus(
			ctx,
			pool,
			nil,
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelUI, "desk-book-position-test"),
			},
		)

		defer func() { _ = desk.Close() }()

		desk.actions.Store("entry-1", &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "BTC/USD",
			Quantity: 1,
		})
		desk.onExecution(user.Execution{
			ClOrdID:     "entry-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Buy),
			ExecType:    "trade",
			OrderStatus: "filled",
			AvgPrice:    100,
			CumQty:      1,
		})

		_, _ = subscriber.Receive(internal.ChannelUI)

		Convey("It should refresh unrealized P/L from L2 book touch without a ticker", func() {
			desk.onBook(&market.BookUpdate{
				Type:   "snapshot",
				Symbol: "BTC/USD",
				Bids:   []market.BookLevel{{Price: 101, Qty: 1}},
				Asks:   []market.BookLevel{{Price: 101.2, Qty: 1}},
			})

			row, receiveErr := subscriber.Receive(internal.ChannelUI)

			So(receiveErr, ShouldBeNil)
			So(row.Type, ShouldEqual, "positions")

			frame, frameOK := row.Value.(PositionMonitorFrame)

			So(frameOK, ShouldBeTrue)
			So(frame.Positions[0].Mark, ShouldEqual, 101)
			So(frame.Positions[0].Unrealized, ShouldEqual, 1)
		})
	})
}

func BenchmarkSymbolBookApplyUpdate(b *testing.B) {
	update := &market.BookUpdate{
		Type: "update",
		Bids: []market.BookLevel{{Price: 100.0, Qty: 2}},
		Asks: []market.BookLevel{{Price: 100.1, Qty: 3}},
	}
	book := newSymbolBook()
	book.applyBookUpdate(&market.BookUpdate{
		Type: "snapshot",
		Bids: []market.BookLevel{{Price: 99.9, Qty: 1}},
		Asks: []market.BookLevel{{Price: 100.2, Qty: 1}},
	})
	now := time.Now().UTC()

	b.ReportAllocs()

	for b.Loop() {
		book.applyBookUpdate(update)
		_, _ = book.quoteSnapshot("BTC/USD", now)
	}
}
