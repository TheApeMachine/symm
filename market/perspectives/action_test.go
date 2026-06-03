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
		viper.Set("market.quote_currency", "EUR")
		defer viper.Set("trading.paper.wallet_eur", 0)
		defer viper.Set("market.quote_currency", "")

		measurement := Measurement{
			Symbol: "BTC/EUR",
			Last:   50_000,
		}

		convey.Convey("When building an entry action", func() {
			action := ActionFromMeasurement(ActionLimit, measurement)

			convey.Convey("It should buy with fee-aware wallet-notional sizing", func() {
				wantQty := entrySpendableNotional(200, entryDefaultMakerFeePct) / 50_000

				convey.So(action.Side, convey.ShouldEqual, trading.Buy)
				convey.So(action.Quantity, convey.ShouldAlmostEqual, wantQty, 0.0000001)
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

func TestEntrySpendableNotional(t *testing.T) {
	convey.Convey("Given wallet notional and maker fee percent", t, func() {
		spendable := entrySpendableNotional(200, entryDefaultMakerFeePct)

		convey.Convey("It should leave headroom for maker fees on the full notional", func() {
			fee := spendable * entryDefaultMakerFeePct / 100
			convey.So(spendable+fee, convey.ShouldAlmostEqual, 200, 0.0001)
		})
	})
}

func TestIsMakerAction(t *testing.T) {
	convey.Convey("Given playbook action types", t, func() {
		convey.Convey("It should treat resting limit entries as maker", func() {
			convey.So(IsMakerAction(ActionLimit), convey.ShouldBeTrue)
			convey.So(IsMakerAction(ActionIceberg), convey.ShouldBeTrue)
		})

		convey.Convey("It should treat market exits as taker", func() {
			convey.So(IsMakerAction(ActionSettlePosition), convey.ShouldBeFalse)
			convey.So(IsMakerAction(ActionStopLoss), convey.ShouldBeFalse)
		})
	})
}

func TestOrderTypeFromActionType(t *testing.T) {
	convey.Convey("Given playbook action types", t, func() {
		convey.Convey("It should map limit actions to Kraken limit orders", func() {
			orderType, err := OrderTypeFromActionType(ActionLimit)

			convey.So(err, convey.ShouldBeNil)
			convey.So(orderType, convey.ShouldEqual, trading.Limit)
		})

		convey.Convey("It should map stop-loss-limit exits to Kraken stop-loss-limit", func() {
			orderType, err := OrderTypeFromActionType(ActionStopLossLimit)

			convey.So(err, convey.ShouldBeNil)
			convey.So(orderType, convey.ShouldEqual, trading.StopLossLimit)
		})

		convey.Convey("It should map trailing stops to Kraken trailing-stop types", func() {
			trailingStop, err := OrderTypeFromActionType(ActionTrailingStop)

			convey.So(err, convey.ShouldBeNil)
			convey.So(trailingStop, convey.ShouldEqual, trading.TrailingStop)

			trailingStopLimit, err := OrderTypeFromActionType(ActionTrailingStopLimit)

			convey.So(err, convey.ShouldBeNil)
			convey.So(trailingStopLimit, convey.ShouldEqual, trading.TrailingStopLimit)
		})

		convey.Convey("It should reject unsupported action types", func() {
			_, err := OrderTypeFromActionType(ActionNone)

			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
