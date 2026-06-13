package market

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestTouchBookSnapshot(t *testing.T) {
	Convey("Given a merged L2 book", t, func() {
		book := NewTouchBook()
		now := time.Now().UTC()

		book.ApplyBookUpdate(&krakenmarket.BookUpdate{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{{Price: 99.9, Qty: 1}},
			Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 2}},
		}, now)

		Convey("It should expose the touch snapshot", func() {
			snapshot, ok := book.Snapshot("BTC/USD")

			So(ok, ShouldBeTrue)
			So(snapshot.Bid, ShouldEqual, 99.9)
			So(snapshot.Ask, ShouldEqual, 100.1)
		})

		Convey("It should refresh touch from bid-side deltas", func() {
			book.ApplyBookUpdate(&krakenmarket.BookUpdate{
				Type: "update",
				Bids: []krakenmarket.BookLevel{{Price: 100.0, Qty: 3}},
			}, now)

			snapshot, ok := book.Snapshot("BTC/USD")

			So(ok, ShouldBeTrue)
			So(snapshot.Bid, ShouldEqual, 100.0)
			So(snapshot.Ask, ShouldEqual, 100.1)
		})
	})
}

func TestTouchBookConcurrentReadWrite(t *testing.T) {
	book := NewTouchBook()
	now := time.Now().UTC()

	book.ApplyBookUpdate(&krakenmarket.BookUpdate{
		Type: "snapshot",
		Bids: []krakenmarket.BookLevel{{Price: 99.9, Qty: 1}},
		Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 2}},
	}, now)

	var waitGroup sync.WaitGroup

	for workerIndex := range 8 {
		waitGroup.Add(2)

		go func(index int) {
			defer waitGroup.Done()

			book.ApplyTicker(&krakenmarket.TickerUpdate{
				Symbol: "BTC/USD",
				Bid:    99.9 + float64(index)*0.01,
				Ask:    100.1 + float64(index)*0.01,
				Last:   100 + float64(index)*0.01,
				BidQty: 1,
				AskQty: 1,
			}, now)
		}(workerIndex)

		go func() {
			defer waitGroup.Done()

			for range 64 {
				_, _ = book.Snapshot("BTC/USD")
			}
		}()
	}

	waitGroup.Wait()
}

func TestTouchRegistryLoad(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a seeded touch registry", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		registry := NewTestTouchRegistry(t, ctx, pool)

		now := time.Now().UTC()
		registry.SeedTouch(TouchSnapshot{
			Symbol:     "ETH/USD",
			Bid:        3000,
			Ask:        3001,
			Last:       3000.5,
			ObservedAt: now,
		})

		Convey("It should return a fresh touch snapshot", func() {
			snapshot, ok := registry.Load("ETH/USD", now)

			So(ok, ShouldBeTrue)
			So(snapshot.Bid, ShouldEqual, 3000)
			So(snapshot.Ask, ShouldEqual, 3001)
		})
	})
}

func BenchmarkTouchBookApplyUpdate(b *testing.B) {
	update := &krakenmarket.BookUpdate{
		Type: "update",
		Bids: []krakenmarket.BookLevel{{Price: 100.0, Qty: 2}},
		Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 3}},
	}
	book := NewTouchBook()
	now := time.Now().UTC()
	book.ApplyBookUpdate(&krakenmarket.BookUpdate{
		Type: "snapshot",
		Bids: []krakenmarket.BookLevel{{Price: 99.9, Qty: 1}},
		Asks: []krakenmarket.BookLevel{{Price: 100.2, Qty: 1}},
	}, now)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		book.ApplyBookUpdate(update, now)
		_, _ = book.Snapshot("BTC/USD")
	}
}
