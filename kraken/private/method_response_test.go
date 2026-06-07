package private

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestHandleMethodResponse(t *testing.T) {
	Convey("Given a tracked outbound add_order", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 4, nil)
		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		sub := raw.Subscribe("test:method-response", 8)

		websocketClient := &WebSocket{
			raw: raw,
			outbound: map[string]outboundOrder{
				"s001": {symbol: "BTC/EUR", side: trading.Buy},
			},
		}

		Convey("A failed add_order ack publishes a derived rejection envelope", func() {
			websocketClient.handleMethodResponse([]byte(
				`{"method":"add_order","success":false,"error":"Insufficient funds","result":{"cl_ord_id":"s001"}}`,
			))

			waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer waitCancel()

			var frame map[string]any

			for frame == nil {
				derived, err := sub.Wait(waitCtx)
				if err != nil {
					t.Fatal("no derived rejection published")
				}

				candidate, ok := derived.Value.(map[string]any)

				if !ok || candidate["channel"] != "executions" {
					continue
				}

				frame = candidate
			}

			So(frame["symbol"], ShouldEqual, "BTC/EUR")
			So(frame["side"], ShouldEqual, "buy")
			So(frame["qty"], ShouldEqual, 0.0)
			So(frame["reason"], ShouldEqual, "Insufficient funds")
			_, stillTracked := websocketClient.outbound["s001"]
			So(stillTracked, ShouldBeFalse)
		})

		Convey("A successful add_order ack leaves pending tracking intact", func() {
			websocketClient.handleMethodResponse([]byte(
				`{"method":"add_order","success":true,"result":{"order_id":"O1","cl_ord_id":"s001"}}`,
			))

			waitCtx, waitCancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer waitCancel()

			if message, err := sub.Wait(waitCtx); err == nil {
				frame, ok := message.Value.(map[string]any)

				if ok && frame["channel"] == "executions" {
					t.Fatal("successful ack should not publish a derived rejection")
				}
			}

			_, stillTracked := websocketClient.outbound["s001"]
			So(stillTracked, ShouldBeTrue)
		})
	})
}

func TestTrackOutbound(t *testing.T) {
	Convey("Given a private websocket", t, func() {
		websocketClient := &WebSocket{outbound: make(map[string]outboundOrder)}

		websocketClient.trackOutbound(map[string]any{
			"method": trading.MethodAddOrder,
			"params": trading.AddParams{
				ClOrdID: "s002",
				Symbol:  "ETH/EUR",
				Side:    trading.Sell,
			},
		})

		order, ok := websocketClient.lookupOutbound("s002")

		So(ok, ShouldBeTrue)
		So(order.symbol, ShouldEqual, "ETH/EUR")
		So(order.side, ShouldEqual, trading.Sell)
	})
}
