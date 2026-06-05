package user

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestPublishExecutionDerived(t *testing.T) {
	Convey("Given a raw broadcast group", t, func() {
		pool := qpool.NewQ(t.Context(), 1, 4, nil)
		raw := pool.CreateBroadcastGroup("raw", 0)
		sub := raw.Subscribe("test:derived", 4)

		Convey("Only trade rows produce derived envelopes", func() {
			PublishExecutionsRaw(raw, "update", []Execution{
				{Symbol: "BTC/EUR", ExecType: "new", Side: "buy"},
				{
					Symbol: "BTC/EUR", ExecType: "trade", Side: "buy",
					LastQty: 1, LastPrice: 100, OrderStatus: "filled",
					Fees: []ExecutionFee{{Asset: "EUR", Qty: 0.4}},
				},
			})

			var derived map[string]any

			select {
			case msg := <-sub.Incoming:
				frame, _ := msg.Value.(map[string]any)

				if frame["qty"] != nil {
					derived = frame
				}
			default:
			}

			select {
			case msg := <-sub.Incoming:
				frame, _ := msg.Value.(map[string]any)

				if frame["qty"] != nil {
					derived = frame
				}
			default:
			}

			So(derived, ShouldNotBeNil)
			So(derived["qty"], ShouldEqual, 1.0)
			So(derived["fee"], ShouldEqual, 0.4)
		})
	})
}
