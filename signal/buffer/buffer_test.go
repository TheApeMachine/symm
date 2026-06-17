package buffer

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestTradeUpdate(testingTB *testing.T) {
	Convey("Given a trade buffer", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 4)
		trade := NewTrade(context.Background())
		now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

		trade.Update(krakenmarket.TradeFeedArtifact(krakenmarket.TradeUpdates{
			{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     50000,
				Qty:       0.5,
				Timestamp: now,
			},
		}))

		Convey("When Snapshot is called", func() {
			snapshot := trade.Snapshot("BTC/USD")

			Convey("It should expose the latest trade facts", func() {
				So(snapshot.Price, ShouldEqual, 50000)
				So(snapshot.Volume, ShouldEqual, 25000)
				So(snapshot.Observed, ShouldEqual, now)
			})
		})

		Convey("When Read streams the scoped element", func() {
			trade.Scope = "BTC/USD"
			trade.ResetReadHead()

			buffer := make([]byte, 4096)
			readCount, readErr := trade.Read(buffer)

			Convey("It should return one marshaled artifact", func() {
				So(readErr, ShouldEqual, io.EOF)
				So(readCount, ShouldBeGreaterThan, 0)

				_, secondErr := trade.Read(buffer)
				So(secondErr, ShouldEqual, io.EOF)
			})
		})
	})
}

func TestBookSpread(testingTB *testing.T) {
	Convey("Given a book buffer", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 4)
		book := NewBook(context.Background())

		book.Update(krakenmarket.BookFeedArtifact(krakenmarket.BookUpdates{
			{
				Symbol: "BTC/USD",
				Bids:   []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
				Asks:   []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
			},
		}))

		Convey("When Spread is called", func() {
			spreadBps := book.Spread("BTC/USD")

			Convey("It should return touch spread in basis points", func() {
				So(spreadBps, ShouldAlmostEqual, 200, 0.01)
			})
		})
	})
}

func TestTickerSnapshot(testingTB *testing.T) {
	Convey("Given a ticker buffer", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 4)
		ticker := NewTicker(context.Background())
		now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

		ticker.Update(krakenmarket.TickerFeedArtifact(krakenmarket.TickerUpdates{
			{
				Symbol:    "BTC/USD",
				Last:      100,
				Bid:       99,
				Ask:       101,
				Volume:    12,
				ChangePct: 0.5,
				Timestamp: now,
			},
		}))

		snapshot := ticker.Snapshot("BTC/USD")

		Convey("It should expose the latest ticker row", func() {
			So(snapshot.Last, ShouldEqual, 100)
			So(snapshot.Bid, ShouldEqual, 99)
			So(snapshot.Ask, ShouldEqual, 101)
			So(snapshot.Volume, ShouldEqual, 12)
			So(snapshot.ChangePct, ShouldEqual, 0.5)
			So(snapshot.Observed, ShouldEqual, now)
		})
	})
}

func BenchmarkTradeUpdate(benchmark *testing.B) {
	viper.Set("signals.feed_ring_capacity", 64)
	trade := NewTrade(context.Background())
	artifact := krakenmarket.TradeFeedArtifact(krakenmarket.TradeUpdates{
		{Symbol: "BTC/USD", Price: 50000, Qty: 1},
	})

	for benchmark.Loop() {
		trade.Update(artifact)
	}
}
