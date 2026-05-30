package order

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestClientDispatchExecutions(t *testing.T) {
	convey.Convey("Given an executions payload", t, func() {
		client := &Client{fills: make(chan Fill, 4)}
		payload := []byte(`{
			"channel": "executions",
			"data": [{
				"order_id": "O1",
				"cl_ord_id": "s1-abc",
				"symbol": "BTC/EUR",
				"side": "buy",
				"exec_type": "trade",
				"exec_id": "E1",
				"last_qty": 0.01,
				"last_price": 100
			}]
		}`)
		client.dispatch(payload)

		convey.Convey("It should enqueue one fill", func() {
			fill := <-client.fills
			convey.So(fill.ClOrdID, convey.ShouldEqual, "s1-abc")
		})
	})
}

func TestClientDispatchAck(t *testing.T) {
	convey.Convey("Given a method ack payload", t, func() {
		client := &Client{acks: make(chan Ack, 2)}
		payload := []byte(`{
			"method": "add_order",
			"success": true,
			"result": {"order_id": "O9", "cl_ord_id": "s1-abc"}
		}`)
		client.dispatch(payload)

		convey.Convey("It should enqueue the ack", func() {
			ack := <-client.acks
			convey.So(ack.Success, convey.ShouldBeTrue)
			convey.So(ack.Result.OrderID, convey.ShouldEqual, "O9")
		})
	})
}
