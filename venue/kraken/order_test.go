package kraken_test

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestOrderWebSocketAckUnmarshal(t *testing.T) {
	Convey("Given a Kraken v2 add_order acknowledgement frame", t, func() {
		raw := []byte(`{
			"method":"add_order",
			"result":{
				"order_id":"O3RGN3-S2VJ7-ZNHVNO",
				"cl_ord_id":"desk-enter-42",
				"order_userref":0
			},
			"success":true,
			"error":"",
			"req_id":901,
			"time_in":"2026-03-06T14:20:01.123456Z",
			"time_out":"2026-03-06T14:20:01.234567Z"
		}`)

		order := kraken.NewOrder()
		So(sonic.Unmarshal(raw, order), ShouldBeNil)

		Convey("It decodes venue identifiers and success", func() {
			So(order.Method, ShouldEqual, "add_order")
			So(order.Success, ShouldBeTrue)
			So(order.Error, ShouldEqual, "")
			So(order.ReqID, ShouldEqual, 901)
			So(order.Result.OrderID, ShouldEqual, "O3RGN3-S2VJ7-ZNHVNO")
			So(order.Result.ClOrdID, ShouldEqual, "desk-enter-42")
			So(order.TimeIn.UTC(), ShouldEqual, time.Date(2026, 3, 6, 14, 20, 1, 123456000, time.UTC))
		})
	})
}

func TestAddOrderRequestCommand(t *testing.T) {
	Convey("Given a limit buy request", t, func() {
		req := &kraken.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			OrdType: "limit",
			Volume:  "0.25",
			Price:   "64000.0",
			ClOrdId: "enter-btc-1",
		}

		Convey("Command carries paper CLI limit params", func() {
			cmd := req.Command()
			So(cmd[0], ShouldEqual, "buy")
			So(cmd[1], ShouldEqual, "BTC/USD")
			So(cmd, ShouldContain, "0.25")
			So(cmd, ShouldContain, "--cl-ord-id")
			So(cmd, ShouldContain, "enter-btc-1")
			So(cmd, ShouldContain, "64000.0")
		})
	})
}
