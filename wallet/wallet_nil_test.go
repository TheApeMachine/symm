package wallet

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalletNilReceivers(t *testing.T) {
	Convey("Given a nil wallet", t, func() {
		var tradingWallet *Wallet

		Convey("It should treat reads as empty and mutations as no-ops", func() {
			So(tradingWallet.SeenFill("x"), ShouldBeFalse)
			tradingWallet.MarkFill("x")
			So(tradingWallet.Snapshot(), ShouldBeNil)
			So(tradingWallet.BalanceCopy(), ShouldEqual, 0)
			So(tradingWallet.InventoryQty("BTC"), ShouldEqual, 0)
			So(tradingWallet.ApplyFill("x", "buy", "BTC", 1, 1, 1), ShouldBeFalse)
			So(tradingWallet.MarkEquity(nil), ShouldEqual, 0)
		})
	})
}

func TestWalletTakeCapsToReserved(t *testing.T) {
	Convey("Given a take larger than reserved cash", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 100, 0.26)
		tradingWallet.ReservedEUR = 10

		err := tradingWallet.Take(25)

		Convey("It should release only the reserved amount", func() {
			So(err, ShouldBeNil)
			So(tradingWallet.Balance, ShouldAlmostEqual, 110, 1e-9)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})
	})
}
