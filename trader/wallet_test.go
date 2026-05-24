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
