package order

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestParseExecutionFills(t *testing.T) {
	convey.Convey("Given a trade execution frame", t, func() {
		payload := []byte(`{
			"channel": "executions",
			"data": [{
				"order_id": "O1",
				"cl_ord_id": "s1-abc",
				"symbol": "BTC/EUR",
				"side": "buy",
				"exec_type": "trade",
				"exec_id": "E1",
				"trade_id": 9,
				"last_qty": 0.01,
				"last_price": 50000
			}]
		}`)
		fills, err := ParseExecutionFills(payload)

		convey.Convey("It should extract one fill", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 1)
			convey.So(fills[0].ExecKey, convey.ShouldEqual, "E1")
			convey.So(fills[0].ClOrdID, convey.ShouldEqual, "s1-abc")
		})
	})
}
