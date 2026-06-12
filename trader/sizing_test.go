package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/logic"
)

func TestEntrySlotFraction(t *testing.T) {
	Convey("Given a five-slot capital envelope", t, func() {
		tradingConfig := config.TradingConfig{
			PrimarySlotCount:       2,
			PrimarySlotFraction:    0.2,
			SecondarySlotFraction:  0.1,
			MaxConcurrentPositions: 5,
		}

		holdings := logic.NewHoldings()
		holdings.SetPosition("AAA/USD", 1, 0.9)
		holdings.SetPosition("BBB/USD", 1, 0.8)

		Convey("It should allocate the primary fraction to a top-tier entry", func() {
			fraction, err := EntrySlotFraction(holdings, 0.85, tradingConfig)

			So(err, ShouldBeNil)
			So(fraction, ShouldEqual, 0.2)
		})

		Convey("It should allocate the secondary fraction to a lower-tier entry", func() {
			fraction, err := EntrySlotFraction(holdings, 0.7, tradingConfig)

			So(err, ShouldBeNil)
			So(fraction, ShouldEqual, 0.1)
		})
	})
}

func TestOrderQuantityFromFraction(t *testing.T) {
	Convey("Given a paper wallet and reference price", t, func() {
		quantity, err := OrderQuantityFromFraction(200, 0.2, 50_000)

		Convey("It should convert notional into base quantity", func() {
			So(err, ShouldBeNil)
			So(quantity, ShouldAlmostEqual, 0.0008, 1e-12)
		})
	})
}

func TestQuoteWalletBalance(t *testing.T) {
	Convey("Given paper wallet config", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		balance, err := QuoteWalletBalance("paper")

		Convey("It should return the configured quote balance", func() {
			So(err, ShouldBeNil)
			So(balance, ShouldEqual, 200)
		})
	})
}

func BenchmarkOrderQuantityFromFraction(b *testing.B) {
	for b.Loop() {
		_, _ = OrderQuantityFromFraction(200, 0.2, 50_000)
	}
}
