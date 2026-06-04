package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestActionFromAct(t *testing.T) {
	convey.Convey("Given a measurement", t, func() {
		measurement := Measurement{
			Symbol: "BTC/EUR",
			Last:   50_000,
		}

		convey.Convey("An entry buys without a preset quantity (the trader sizes it)", func() {
			action := ActionFromAct(Act{Type: ActionLimit}, measurement)

			convey.So(action.Side, convey.ShouldEqual, trading.Buy)
			convey.So(action.Symbol, convey.ShouldEqual, "BTC/EUR")
			convey.So(action.Price, convey.ShouldEqual, 50_000)
			convey.So(action.Quantity, convey.ShouldEqual, 0)
		})

		convey.Convey("An exit sells and carries the per-node trigger offset", func() {
			action := ActionFromAct(Act{Type: ActionTrailingStop, Offset: 0.02}, measurement)

			convey.So(action.Side, convey.ShouldEqual, trading.Sell)
			convey.So(action.Quantity, convey.ShouldEqual, 0)
			convey.So(action.Offset, convey.ShouldEqual, 0.02)
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
