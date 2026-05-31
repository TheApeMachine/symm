package broker

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSellAggressorHitsBid(t *testing.T) {
	Convey("Given a buy-aggressor trade", t, func() {
		hits := SellAggressorHitsBid(market.TradeUpdate{
			Side:  "buy",
			Price: 50000,
			Qty:   1,
		}, 50000)

		Convey("It should not count toward bid queue turnover", func() {
			So(hits, ShouldBeFalse)
		})
	})

	Convey("Given a sell-aggressor trade above the limit", t, func() {
		hits := SellAggressorHitsBid(market.TradeUpdate{
			Side:  "sell",
			Price: 50100,
			Qty:   1,
		}, 50000)

		Convey("It should not count because price traded elsewhere", func() {
			So(hits, ShouldBeFalse)
		})
	})

	Convey("Given a sell-aggressor trade at the limit", t, func() {
		hits := SellAggressorHitsBid(market.TradeUpdate{
			Side:  "sell",
			Price: 50000,
			Qty:   1,
		}, 50000)

		Convey("It should count toward bid queue turnover", func() {
			So(hits, ShouldBeTrue)
		})
	})

	Convey("Given a sell-aggressor trade through the limit", t, func() {
		hits := SellAggressorHitsBid(market.TradeUpdate{
			Side:  "sell",
			Price: 49950,
			Qty:   1,
		}, 50000)

		Convey("It should count because bids at or above the print were hit", func() {
			So(hits, ShouldBeTrue)
		})
	})
}

func TestMakerQueueTrackerPostTime(t *testing.T) {
	Convey("Given a resting bid posted at t0", t, func() {
		postedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
		tracker := NewMakerQueueTracker("BTC/EUR", 50000, postedAt, nil)

		tracker.ObserveTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "sell",
			Price:     50000,
			Qty:       2,
			Timestamp: postedAt.Add(-time.Second),
		})

		Convey("It should ignore sell-aggressor volume before post time", func() {
			So(tracker.BidTradeVolume, ShouldEqual, 0)
		})

		tracker.ObserveTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "sell",
			Price:     50000,
			Qty:       0.4,
			Timestamp: postedAt,
		})

		Convey("It should accumulate volume only after post time", func() {
			So(tracker.BidTradeVolume, ShouldEqual, 0.4)
		})
	})
}

func TestMakerQueueTrackerPartialQueueNotEnough(t *testing.T) {
	Convey("Given queue ahead exceeds post-post sell volume", t, func() {
		postedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
		tracker := NewMakerQueueTracker("BTC/EUR", 50000, postedAt, []market.BookLevel{
			{Price: 50000, Qty: 1.5},
		})

		tracker.ObserveTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "sell",
			Price:     50000,
			Qty:       1.4,
			Timestamp: postedAt.Add(time.Second),
		})

		queue := tracker.Context()

		Convey("It should not be fill-ready", func() {
			So(MakerFillReady(queue, 50000, 0.1), ShouldBeFalse)
		})
	})
}

func TestMakerQueueTrackerFreezesQueueAheadAtPost(t *testing.T) {
	Convey("Given queue ahead snapshotted when the bid was posted", t, func() {
		postedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
		tracker := NewMakerQueueTracker("BTC/EUR", 50000, postedAt, []market.BookLevel{
			{Price: 50000, Qty: 2},
		})

		tracker.ObserveTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "sell",
			Price:     50000,
			Qty:       0.15,
			Timestamp: postedAt.Add(time.Second),
		})

		queue := tracker.Context()

		Convey("It should not treat later bid cancels as queue consumption", func() {
			So(queue.InitialQueueAheadBaseQty, ShouldEqual, 2)
			So(MakerFillReady(queue, 50000, 0.1), ShouldBeFalse)
		})
	})
}

func TestMakerQueueTrackerNoFillBeforeQueueConsumed(t *testing.T) {
	Convey("Given a maker bid backed by tracked sell-aggressor volume", t, func() {
		originalPenalty := config.System.AdverseSelectionBPS
		config.System.AdverseSelectionBPS = 5
		t.Cleanup(func() { config.System.AdverseSelectionBPS = originalPenalty })

		postedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
		tracker := NewMakerQueueTracker("BTC/EUR", 50000, postedAt, []market.BookLevel{
			{Price: 50000, Qty: 1},
		})

		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
		maker := &Maker{Symbol: "BTC/EUR", LimitPrice: 50000, Notional: 10}

		_, err := maker.SubmitPaper(tradingWallet)

		if err != nil {
			t.Fatalf("submit paper: %v", err)
		}

		_, buildErr := maker.BuildPaperFill(tracker.Context())

		Convey("It should reject before queue turnover", func() {
			So(buildErr, ShouldEqual, ErrMakerQueueNotReady)
			So(tradingWallet.ReservedEUR, ShouldEqual, 10)
		})

		tracker.ObserveTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "sell",
			Price:     50000,
			Qty:       1.05,
			Timestamp: postedAt.Add(time.Second),
		})

		fill, fillErr := maker.BuildPaperFill(tracker.Context())

		if fillErr != nil {
			t.Fatalf("build fill: %v", fillErr)
		}

		if settleErr := tradingWallet.SettleEntryReservation(maker.Notional, maker.Notional); settleErr != nil {
			t.Fatalf("settle: %v", settleErr)
		}

		tradingWallet.AddInventoryWithCost("BTC", fill.Qty, maker.Notional)

		Convey("It should fill once sell-aggressor volume clears the queue", func() {
			So(fill.Qty, ShouldBeGreaterThan, 0)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})
	})
}

func TestMakerQueueTrackerIgnoresOffLimitTape(t *testing.T) {
	Convey("Given trades that do not hit our bid level", t, func() {
		postedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
		tracker := NewMakerQueueTracker("BTC/EUR", 50000, postedAt, []market.BookLevel{
			{Price: 50000, Qty: 1},
		})

		tracker.ObserveTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "buy",
			Price:     50050,
			Qty:       3,
			Timestamp: postedAt.Add(time.Second),
		})
		tracker.ObserveTrade(market.TradeUpdate{
			Symbol:    "BTC/EUR",
			Side:      "sell",
			Price:     50100,
			Qty:       3,
			Timestamp: postedAt.Add(2 * time.Second),
		})

		Convey("It should not accumulate bid-hit volume", func() {
			So(tracker.BidTradeVolume, ShouldEqual, 0)
			So(MakerFillReady(tracker.Context(), 50000, 0.1), ShouldBeFalse)
		})
	})
}

func BenchmarkMakerQueueTrackerObserveTrade(b *testing.B) {
	postedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	tracker := NewMakerQueueTracker("BTC/EUR", 50000, postedAt, nil)
	trade := market.TradeUpdate{
		Symbol:    "BTC/EUR",
		Side:      "sell",
		Price:     49990,
		Qty:       0.25,
		Timestamp: postedAt.Add(time.Second),
	}

	b.ReportAllocs()

	for b.Loop() {
		tracker.ObserveTrade(trade)
	}
}
