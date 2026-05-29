package wallet

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalletSeenFill(t *testing.T) {
	Convey("Given a wallet with fill deduplication", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

		Convey("It should treat empty exec keys as never seen", func() {
			So(tradingWallet.SeenFill(""), ShouldBeFalse)
		})

		Convey("It should report unseen keys until marked", func() {
			So(tradingWallet.SeenFill("exec-1"), ShouldBeFalse)
			tradingWallet.MarkFill("exec-1")
			So(tradingWallet.SeenFill("exec-1"), ShouldBeTrue)
		})

		Convey("It should ignore duplicate marks for the same key", func() {
			tradingWallet.MarkFill("exec-2")
			tradingWallet.MarkFill("exec-2")
			So(tradingWallet.SeenFill("exec-2"), ShouldBeTrue)
		})
	})
}

func TestWalletApplyFillBuy(t *testing.T) {
	Convey("Given a wallet applying a buy fill", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

		applied := tradingWallet.ApplyFill("buy-1", "buy", "BTC", 0.01, 50000, 500)

		Convey("It should credit inventory and record cost basis from cash delta", func() {
			So(applied, ShouldBeTrue)
			So(tradingWallet.InventoryQty("BTC"), ShouldAlmostEqual, 0.01, 1e-12)
			So(tradingWallet.AvgEntryFor("BTC"), ShouldAlmostEqual, 50000, 1e-9)
		})

		Convey("It should reject duplicate exec keys", func() {
			again := tradingWallet.ApplyFill("buy-1", "buy", "BTC", 0.01, 50000, 500)
			So(again, ShouldBeFalse)
			So(tradingWallet.InventoryQty("BTC"), ShouldAlmostEqual, 0.01, 1e-12)
		})
	})
}

func TestWalletApplyFillSell(t *testing.T) {
	Convey("Given a wallet with BTC inventory", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
		tradingWallet.AddInventoryWithCost("BTC", 0.02, 1000)

		Convey("It should debit partial inventory and credit cash", func() {
			applied := tradingWallet.ApplyFill("sell-1", "sell", "BTC", 0.01, 52000, 520)
			So(applied, ShouldBeTrue)
			So(tradingWallet.InventoryQty("BTC"), ShouldAlmostEqual, 0.01, 1e-12)
			So(tradingWallet.Balance, ShouldAlmostEqual, 720, 1e-9)
			So(tradingWallet.AvgEntryFor("BTC"), ShouldBeGreaterThan, 0)
		})

		Convey("It should clear position economics on a full exit", func() {
			dueAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			tradingWallet.BindPosition("BTC", PositionBinding{
				Source:      "perspective:flow",
				PredictedAt: dueAt.Add(-time.Minute),
				DueAt:       dueAt,
			})
			applied := tradingWallet.ApplyFill("sell-all", "sell", "BTC", 0.02, 52000, 1040)
			So(applied, ShouldBeTrue)
			So(tradingWallet.InventoryQty("BTC"), ShouldEqual, 0)
			So(tradingWallet.AvgEntryFor("BTC"), ShouldEqual, 0)
			_, bound := tradingWallet.PositionBindingFor("BTC")
			So(bound, ShouldBeFalse)
		})

		Convey("It should cap inventory at zero when sell exceeds tracked qty", func() {
			applied := tradingWallet.ApplyFill("oversell", "sell", "BTC", 0.05, 52000, 2600)
			So(applied, ShouldBeTrue)
			So(tradingWallet.InventoryQty("BTC"), ShouldEqual, 0)
			So(tradingWallet.Balance, ShouldAlmostEqual, 2800, 1e-9)
		})
	})
}

func BenchmarkWalletApplyFill(b *testing.B) {
	tradingWallet := NewWallet(PaperWallet, "EUR", 1e9, 0.26)

	for b.Loop() {
		tradingWallet.ApplyFill("", "buy", "BTC", 0.001, 50000, 50)
	}
}

func BenchmarkWalletSeenFill(b *testing.B) {
	tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	tradingWallet.MarkFill("warm")

	for b.Loop() {
		tradingWallet.SeenFill("warm")
	}
}
