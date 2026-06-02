package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestActionFromMeasurement(t *testing.T) {
	convey.Convey("Given a configured paper wallet", t, func() {
		viper.Set("trading.paper.wallet_eur", 200.0)
		defer viper.Set("trading.paper.wallet_eur", 0)

		measurement := Measurement{
			Symbol: "BTC/EUR",
			Last:   50_000,
		}

		convey.Convey("When building an entry action", func() {
			action := ActionFromMeasurement(ActionLimit, measurement)

			convey.Convey("It should buy with wallet-notional sizing", func() {
				convey.So(action.Side, convey.ShouldEqual, trading.Buy)
				convey.So(action.Quantity, convey.ShouldAlmostEqual, 200.0/50_000, 0.0000001)
			})
		})

		convey.Convey("When building an exit action", func() {
			action := ActionFromMeasurement(ActionStopLoss, measurement)

			convey.Convey("It should sell without preset quantity", func() {
				convey.So(action.Side, convey.ShouldEqual, trading.Sell)
				convey.So(action.Quantity, convey.ShouldEqual, 0)
			})
		})
	})
}
