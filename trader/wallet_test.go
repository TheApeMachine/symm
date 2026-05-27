package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestWalletReserveEntry(t *testing.T) {
	convey.Convey("Given a wallet with available cash", t, func() {
		wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

		convey.Convey("It should move cash into reserved on entry reservation", func() {
			err := wallet.ReserveEntry(20)
			convey.So(err, convey.ShouldBeNil)
			convey.So(wallet.AvailableEUR(), convey.ShouldEqual, 180)
			convey.So(wallet.ReservedEUR, convey.ShouldEqual, 20)
		})

		convey.Convey("It should settle reserved cash against actual fill cost", func() {
			err := wallet.ReserveEntry(20)
			convey.So(err, convey.ShouldBeNil)

			err = wallet.SettleEntryReservation(20, 19.5)
			convey.So(err, convey.ShouldBeNil)
			convey.So(wallet.ReservedEUR, convey.ShouldEqual, 0)
			convey.So(wallet.AvailableEUR(), convey.ShouldEqual, 180.5)
		})

		convey.Convey("It should release reservation after failed entry", func() {
			err := wallet.ReserveEntry(20)
			convey.So(err, convey.ShouldBeNil)

			wallet.ReleaseEntryReservation(20)
			convey.So(wallet.AvailableEUR(), convey.ShouldEqual, 200)
			convey.So(wallet.ReservedEUR, convey.ShouldEqual, 0)
		})
	})
}

func TestWalletRecordFill(t *testing.T) {
	convey.Convey("Given a wallet with inventory", t, func() {
		convey.Convey("It should track average entry on first fill", func() {
			wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
			wallet.Inventory["BTC"] = 1
			wallet.RecordFill("BTC", 1, 50000)
			convey.So(wallet.AvgEntry["BTC"], convey.ShouldEqual, 50000)
		})

		convey.Convey("It should volume-weight subsequent fills", func() {
			wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
			wallet.Inventory["BTC"] = 3
			wallet.AvgEntry["BTC"] = 50000
			wallet.RecordFill("BTC", 2, 52000)
			convey.So(wallet.AvgEntry["BTC"], convey.ShouldAlmostEqual, 51333.33333333333, 0.0001)
		})

		convey.Convey("It should clear entry economics on exit", func() {
			wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
			wallet.AvgEntry["BTC"] = 50000
			wallet.ClearPosition("BTC")
			_, ok := wallet.AvgEntry["BTC"]
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}

func TestWalletSnapshot(t *testing.T) {
	convey.Convey("Given a wallet with mutable maps", t, func() {
		wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
		wallet.Inventory["BTC"] = 0.01
		wallet.AvgEntry["BTC"] = 50000
		wallet.Marks = map[string]float64{"BTC/EUR": 50100}

		snapshot := wallet.Snapshot()

		wallet.Inventory["BTC"] = 0.02
		wallet.AvgEntry["BTC"] = 51000
		wallet.Marks["BTC/EUR"] = 50200

		convey.Convey("It should copy scalar and map state", func() {
			convey.So(snapshot, convey.ShouldNotEqual, wallet)
			convey.So(snapshot.Inventory["BTC"], convey.ShouldEqual, 0.01)
			convey.So(snapshot.AvgEntry["BTC"], convey.ShouldEqual, 50000)
			convey.So(snapshot.Marks["BTC/EUR"], convey.ShouldEqual, 50100)
		})
	})
}

func BenchmarkWalletSnapshot(b *testing.B) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.Inventory["BTC"] = 0.01
	wallet.Inventory["ETH"] = 0.2
	wallet.AvgEntry["BTC"] = 50000
	wallet.AvgEntry["ETH"] = 3000
	wallet.Marks = map[string]float64{
		"BTC/EUR": 50100,
		"ETH/EUR": 3010,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = wallet.Snapshot()
	}
}
