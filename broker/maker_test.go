package broker

import (
	"testing"

	"github.com/theapemachine/symm/kraken/order"
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

func TestMakerSubmitLiveRoundsLimitPrice(t *testing.T) {
	Convey("Given a live maker bid with price precision", t, func() {
		tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 200, 0.26)
		orders := make([]any, 0, 1)
		router := NewRouter(func(value any) { orders = append(orders, value) })

		err := (&Maker{
			Symbol:           "BTC/EUR",
			LimitPrice:       50000.129,
			Notional:         10,
			HasPriceDecimals: true,
			PriceDecimals:    2,
		}).SubmitLive(router, tradingWallet)

		Convey("It should floor the price before publishing", func() {
			So(err, ShouldBeNil)
			So(orders, ShouldHaveLength, 1)
			So(orders[0].(order.Request).Params.LimitPrice, ShouldEqual, 50000.12)
		})
	})
}

func TestMakerSubmitLiveRequiresPriceDecimals(t *testing.T) {
	Convey("Given a live maker bid without price precision", t, func() {
		tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 200, 0.26)
		orders := make([]any, 0, 1)
		router := NewRouter(func(value any) { orders = append(orders, value) })

		err := (&Maker{
			Symbol:     "BTC/EUR",
			LimitPrice: 50000.129,
			Notional:   10,
		}).SubmitLive(router, tradingWallet)

		Convey("It should release the reservation and avoid publishing", func() {
			So(err, ShouldNotBeNil)
			So(orders, ShouldHaveLength, 0)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})
	})
}

func BenchmarkMakerSubmitLiveRounded(b *testing.B) {
	router := NewRouter(func(value any) {})
	maker := &Maker{
		Symbol:           "BTC/EUR",
		LimitPrice:       50000.129,
		Notional:         10,
		HasPriceDecimals: true,
		PriceDecimals:    2,
	}

	b.ReportAllocs()

	for b.Loop() {
		tradingWallet := wallet.NewWallet(wallet.CryptoWallet, "EUR", 200, 0.26)

		if err := maker.SubmitLive(router, tradingWallet); err != nil {
			b.Fatal(err)
		}
	}
}
