package paper

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestWebSocket_Send(t *testing.T) {
	convey.Convey("Given a paper websocket", t, func() {
		ctx := context.Background()

		ws, err := NewWebSocket(ctx)
		convey.So(err, convey.ShouldBeNil)

		var frames [][]byte

		ws.OnMessage(func(payload []byte) {
			frames = append(frames, append([]byte(nil), payload...))
		})

		err = ws.Send(public.OrdersChannel, map[string]any{
			"method": "add_order",
			"params": map[string]any{
				"order_type":  "limit",
				"side":        "buy",
				"symbol":      "BTC/USD",
				"order_qty":   0.01,
				"limit_price": 50000.0,
				"cl_ord_id":   "paper-test-001",
			},
		})

		convey.Convey("It should emit add_order ack and execution trade frames", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(frames), convey.ShouldEqual, 2)
		})
	})
}
