package broker

import (
	"testing"

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
