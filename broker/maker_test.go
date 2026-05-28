package broker

import (
	"testing"

	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMakerFillPaper(t *testing.T) {
	Convey("Given a reserved maker entry", t, func() {
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)

		if err := tradingWallet.ReserveEntry(10); err != nil {
			t.Fatalf("reserve: %v", err)
		}

		fill, err := (&Maker{
			Symbol:     "BTC/EUR",
			LimitPrice: 50000,
			Notional:   10,
		}).FillPaper(tradingWallet)

		Convey("It should fill at the limit", func() {
			So(err, ShouldBeNil)
			So(fill.Price, ShouldEqual, 50000)
			So(tradingWallet.Inventory["BTC"], ShouldBeGreaterThan, 0)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})
	})
}
