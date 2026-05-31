package broker

import (
	"testing"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuyFillPaper(t *testing.T) {
	Convey("Given a paper buy", t, func() {
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)

		fill, err := (&Buy{
			Symbol:   "BTC/EUR",
			Notional: 10,
			Quote: Quote{
				Last: 50000,
				Bid:  49999,
				Ask:  50001,
			},
		}).FillPaper(tradingWallet)

		Convey("It should fill inventory and spend the notional", func() {
			So(err, ShouldBeNil)
			So(fill.Qty, ShouldBeGreaterThan, 0)
			So(tradingWallet.Inventory["BTC"], ShouldEqual, fill.Qty)
			So(tradingWallet.Balance, ShouldEqual, 190)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})
	})
}

func TestBuySubmitLive(t *testing.T) {
	Convey("Given a live buy", t, func() {
		tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 200, 0.26)
		orders := make([]any, 0, 1)
		router := NewRouter(func(value any) error { orders = append(orders, value); return nil })

		err := (&Buy{
			Symbol:   "BTC/EUR",
			Notional: 10,
			Quote: Quote{
				Last: 50000,
				Bid:  49999,
				Ask:  50001,
			},
		}).SubmitLive(router, tradingWallet)

		Convey("It should route a market buy", func() {
			So(err, ShouldBeNil)
			So(orders[0].(order.Request).Params.CashOrderQty, ShouldEqual, 10)
			So(tradingWallet.ReservedEUR, ShouldEqual, 10)
		})
	})
}

func TestBuySubmitLiveUsesExistingClOrdID(t *testing.T) {
	Convey("Given a live buy with a client order id", t, func() {
		tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 200, 0.26)
		orders := make([]any, 0, 1)
		router := NewRouter(func(value any) error { orders = append(orders, value); return nil })

		err := (&Buy{
			Symbol:   "BTC/EUR",
			Notional: 10,
			Quote: Quote{
				Last: 50000,
				Bid:  49999,
				Ask:  50001,
			},
			ClOrdID: "CLIENT-1",
		}).SubmitLive(router, tradingWallet)

		Convey("It should route the supplied cl_ord_id", func() {
			So(err, ShouldBeNil)
			So(orders[0].(order.Request).Params.ClOrdID, ShouldEqual, "CLIENT-1")
		})
	})
}

func BenchmarkBuyFillPaper(b *testing.B) {
	buy := &Buy{
		Symbol:   "BTC/EUR",
		Notional: 10,
		Quote: Quote{
			Last: 50000,
			Bid:  49999,
			Ask:  50001,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, config.System.TakerFeePct)
		_, _ = buy.FillPaper(tradingWallet)
	}
}
