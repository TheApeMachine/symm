package paper

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalletApplyFill(t *testing.T) {
	Convey("Given a paper wallet funded in EUR", t, func() {
		wallet := NewWallet("EUR", 200)

		Convey("When a buy fill is applied", func() {
			updates := wallet.ApplyFill("BTC/EUR", "buy", 0.01, 10000, 0.4, "TRADE-1")

			Convey("It should emit ledger updates for base and quote", func() {
				So(len(updates), ShouldEqual, 2)
				So(updates[0].Asset, ShouldEqual, "BTC")
				So(updates[0].Type, ShouldEqual, "trade")
				So(updates[1].Asset, ShouldEqual, "EUR")
				So(updates[1].Fee, ShouldEqual, 0.4)
			})

			Convey("It should reflect the post-trade balances in a snapshot", func() {
				snapshot := wallet.Snapshot()

				So(len(snapshot), ShouldEqual, 2)
				So(snapshot[0].Balance+snapshot[1].Balance, ShouldBeGreaterThan, 0)
			})
		})
	})
}
