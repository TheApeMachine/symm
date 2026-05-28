package broker

import (
	"testing"

	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSellFillPaper(t *testing.T) {
	Convey("Given an open paper position", t, func() {
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 180, 0.26)
		tradingWallet.Inventory["BTC"] = 0.001

		fill, err := (&Sell{
			Symbol: "BTC/EUR",
			Quote: Quote{
				Last: 50000,
				Bid:  49999,
				Ask:  50001,
			},
		}).FillPaper(tradingWallet)

		Convey("It should flatten inventory and credit proceeds", func() {
			So(err, ShouldBeNil)
			So(fill.Qty, ShouldEqual, 0.001)
			So(tradingWallet.Inventory["BTC"], ShouldEqual, 0)
			So(tradingWallet.Balance, ShouldBeGreaterThan, 180)
		})
	})
}

func TestSellSubmitLiveRoundsBaseQuantity(t *testing.T) {
	Convey("Given a live sell with lot decimals", t, func() {
		tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 180, 0.26)
		tradingWallet.Inventory["BTC"] = 0.010204081632
		orders := make([]any, 0, 1)
		router := NewRouter(func(value any) { orders = append(orders, value) })

		err := (&Sell{
			Symbol:         "BTC/EUR",
			HasLotDecimals: true,
			LotDecimals:    4,
		}).SubmitLive(router, tradingWallet)

		Convey("It should floor to the exchange lot precision", func() {
			So(err, ShouldBeNil)
			So(orders, ShouldHaveLength, 1)
			So(orders[0].(order.Request).Params.OrderQty, ShouldEqual, 0.0102)
		})
	})
}

func TestSellSubmitLiveRequiresLotDecimals(t *testing.T) {
	Convey("Given a live sell without instrument precision", t, func() {
		tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 180, 0.26)
		tradingWallet.Inventory["BTC"] = 0.010204081632
		orders := make([]any, 0, 1)
		router := NewRouter(func(value any) { orders = append(orders, value) })

		err := (&Sell{Symbol: "BTC/EUR"}).SubmitLive(router, tradingWallet)

		Convey("It should fail before publishing an invalid order", func() {
			So(err, ShouldNotBeNil)
			So(orders, ShouldHaveLength, 0)
		})
	})
}

func BenchmarkSellFillPaper(b *testing.B) {
	sell := &Sell{
		Symbol: "BTC/EUR",
		Quote: Quote{
			Last: 50000,
			Bid:  49999,
			Ask:  50001,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 180, 0.26)
		tradingWallet.Inventory["BTC"] = 0.001
		_, _ = sell.FillPaper(tradingWallet)
	}
}
