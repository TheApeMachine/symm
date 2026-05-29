package wallet

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalletAddInventory(t *testing.T) {
	Convey("Given inventory operations", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

		tradingWallet.AddInventory("BTC", 0.01, 40000)
		tradingWallet.AddInventoryWithCost("ETH", 0.5, 1500)

		Convey("It should track quantities and average entries", func() {
			So(tradingWallet.InventoryQty("BTC"), ShouldAlmostEqual, 0.01, 1e-12)
			So(tradingWallet.AvgEntryFor("BTC"), ShouldAlmostEqual, 40000, 1e-9)
			So(tradingWallet.InventoryQty("ETH"), ShouldAlmostEqual, 0.5, 1e-12)
			So(tradingWallet.AvgEntryFor("ETH"), ShouldAlmostEqual, 3000, 1e-9)
		})

		Convey("It should volume-weight additional fills", func() {
			tradingWallet.AddInventory("BTC", 0.01, 40000)
			tradingWallet.RecordFill("BTC", 0.01, 60000)
			So(tradingWallet.AvgEntryFor("BTC"), ShouldAlmostEqual, 50000, 1e-9)
		})

		Convey("It should copy inventory for external iteration", func() {
			copy := tradingWallet.InventoryCopy()
			So(copy["BTC"], ShouldAlmostEqual, 0.01, 1e-12)
			copy["BTC"] = 0
			So(tradingWallet.InventoryQty("BTC"), ShouldAlmostEqual, 0.01, 1e-12)
		})
	})
}

func TestWalletZeroInventory(t *testing.T) {
	Convey("Given held inventory", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
		tradingWallet.AddInventory("BTC", 0.03, 42000)
		dueAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
		tradingWallet.BindPosition("BTC", PositionBinding{
			Source:      "perspective:flow",
			PredictedAt: dueAt.Add(-time.Minute),
			DueAt:       dueAt,
		})

		qty := tradingWallet.ZeroInventory("BTC")

		Convey("It should return prior quantity and clear economics", func() {
			So(qty, ShouldAlmostEqual, 0.03, 1e-12)
			So(tradingWallet.InventoryQty("BTC"), ShouldEqual, 0)
			So(tradingWallet.AvgEntryFor("BTC"), ShouldEqual, 0)
			_, bound := tradingWallet.PositionBindingFor("BTC")
			So(bound, ShouldBeFalse)
		})
	})
}

func TestWalletSnapshot(t *testing.T) {
	Convey("Given a wallet with state", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 180, 0.26)
		_ = tradingWallet.Reserve(20)
		tradingWallet.AddInventory("BTC", 0.01, 50000)
		tradingWallet.SetMarks(map[string]float64{"BTC/EUR": 51000})

		snapshot := tradingWallet.Snapshot()

		Convey("It should detach a consistent copy", func() {
			So(snapshot, ShouldNotBeNil)
			So(snapshot.Balance, ShouldAlmostEqual, 160, 1e-9)
			So(snapshot.ReservedEUR, ShouldAlmostEqual, 20, 1e-9)
			So(snapshot.Inventory["BTC"], ShouldAlmostEqual, 0.01, 1e-12)
			So(snapshot.Marks["BTC/EUR"], ShouldAlmostEqual, 51000, 1e-9)
		})

		Convey("It should expose balance and reserved copies under lock", func() {
			So(tradingWallet.BalanceCopy(), ShouldAlmostEqual, 160, 1e-9)
			So(tradingWallet.ReservedCopy(), ShouldAlmostEqual, 20, 1e-9)
			So(tradingWallet.AvailableEUR(), ShouldAlmostEqual, 160, 1e-9)
		})
	})
}

func TestWalletMarkEquity(t *testing.T) {
	Convey("Given cash, reservations, and inventory", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 100, 0.26)
		_ = tradingWallet.Reserve(50)
		tradingWallet.AddInventory("BTC", 0.01, 40000)

		equity := tradingWallet.MarkEquity(map[string]float64{"BTC/EUR": 50000})

		Convey("It should mark cash, reserved entry cash, and inventory", func() {
			So(equity, ShouldAlmostEqual, 600, 1e-9)
		})
	})
}

func TestWalletCreditBalanceAndClearPosition(t *testing.T) {
	Convey("Given balance and position helpers", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 100, 0.26)
		tradingWallet.AddInventory("BTC", 0.01, 40000)

		tradingWallet.CreditBalance(25)
		tradingWallet.ClearPosition("BTC")

		Convey("It should apply signed balance deltas and clear entry tracking", func() {
			So(tradingWallet.Balance, ShouldAlmostEqual, 125, 1e-9)
			So(tradingWallet.AvgEntryFor("BTC"), ShouldEqual, 0)
		})
	})
}

func BenchmarkWalletSnapshot(b *testing.B) {
	tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	tradingWallet.AddInventory("BTC", 0.01, 50000)
	tradingWallet.SetMarks(map[string]float64{"BTC/EUR": 51000})

	for b.Loop() {
		_ = tradingWallet.Snapshot()
	}
}

func BenchmarkWalletMarkEquity(b *testing.B) {
	tradingWallet := NewWallet(PaperWallet, "EUR", 100, 0.26)
	tradingWallet.AddInventory("BTC", 0.01, 40000)
	marks := map[string]float64{"BTC/EUR": 50000}

	for b.Loop() {
		tradingWallet.MarkEquity(marks)
	}
}
