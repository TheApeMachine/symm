package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestKrakenOrderTypeDiscretionaryExit(t *testing.T) {
	Convey("Given a story soft exit action", t, func() {
		orderType, err := krakenOrderType(&logic.Action{
			Type: logic.ActionTakeProfit,
		}, false)

		So(err, ShouldBeNil)
		So(orderType, ShouldEqual, trading.Market)
	})

	Convey("Given a native stop-loss action", t, func() {
		orderType, err := krakenOrderType(&logic.Action{
			Type: logic.ActionStopLoss,
		}, false)

		So(err, ShouldBeNil)
		So(orderType, ShouldEqual, trading.StopLoss)
	})
}
