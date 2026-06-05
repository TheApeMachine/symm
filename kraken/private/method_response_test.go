package private

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestHandleMethodResponse(t *testing.T) {
	Convey("Given a tracked outbound add_order", t, func() {
		pool := qpool.NewQ(t.Context(), 1, 4, nil)
		raw := pool.CreateBroadcastGroup("raw", 0)
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

			var derived map[string]any

			deadline := time.After(2 * time.Second)

			for derived == nil {
				select {
				case message := <-sub.Incoming:
					frame, ok := message.Value.(map[string]any)

					if !ok || frame["channel"] != "executions" {
						continue
					}

					derived = frame
				case <-deadline:
					t.Fatal("no derived rejection published")
				}
			}

			So(derived["symbol"], ShouldEqual, "BTC/EUR")
			So(derived["side"], ShouldEqual, "buy")
			So(derived["qty"], ShouldEqual, 0.0)
			So(derived["reason"], ShouldEqual, "Insufficient funds")
			_, stillTracked := websocketClient.outbound["s001"]
			So(stillTracked, ShouldBeFalse)
		})

		Convey("A successful add_order ack leaves pending tracking intact", func() {
			websocketClient.handleMethodResponse([]byte(
				`{"method":"add_order","success":true,"result":{"order_id":"O1","cl_ord_id":"s001"}}`,
			))

			deadline := time.After(100 * time.Millisecond)

			for {
				select {
				case message := <-sub.Incoming:
					frame, ok := message.Value.(map[string]any)

					if ok && frame["channel"] == "executions" {
						t.Fatal("successful ack should not publish a derived rejection")
					}
				case <-deadline:
					goto done
				}
			}

		done:
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
