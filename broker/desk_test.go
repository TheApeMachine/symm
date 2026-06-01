package broker

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestDesk_paramsFromAction(t *testing.T) {
	convey.Convey("Given a desk", t, func() {
		desk := &Desk{}

		convey.Convey("When the action is enter", func() {
			params, err := desk.paramsFromAction(perspectives.Action{
				Type:     perspectives.ActionEnter,
				Symbol:   "BTC/USD",
				Price:    50000,
				Quantity: 0.01,
			})

			convey.Convey("It should build a limit buy", func() {
				convey.So(err, convey.ShouldBeNil)
				convey.So(params.OrderType, convey.ShouldEqual, trading.Limit)
				convey.So(params.Side, convey.ShouldEqual, trading.Buy)
				convey.So(params.LimitPrice, convey.ShouldEqual, 50000)
			})
		})

		convey.Convey("When the action is stop loss without price", func() {
			_, err := desk.paramsFromAction(perspectives.Action{
				Type:     perspectives.ActionStopLoss,
				Symbol:   "BTC/USD",
				Quantity: 0.01,
			})

			convey.Convey("It should error", func() {
				convey.So(err, convey.ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkDesk_paramsFromAction(b *testing.B) {
	desk := &Desk{}
	action := perspectives.Action{
		Type:     perspectives.ActionEnter,
		Symbol:   "BTC/USD",
		Price:    50000,
		Quantity: 0.01,
	}

	for b.Loop() {
		_, _ = desk.paramsFromAction(action)
	}
}
