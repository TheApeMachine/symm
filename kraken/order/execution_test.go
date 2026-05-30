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

		convey.Convey("It should extract one fill with mapped fields", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 1)
			convey.So(fills[0].ExecKey, convey.ShouldEqual, "E1")
			convey.So(fills[0].ClOrdID, convey.ShouldEqual, "s1-abc")
			convey.So(fills[0].Symbol, convey.ShouldEqual, "BTC/EUR")
			convey.So(fills[0].Side, convey.ShouldEqual, "buy")
			convey.So(fills[0].Qty, convey.ShouldEqual, 0.01)
			convey.So(fills[0].Price, convey.ShouldEqual, 50000)
		})
	})

	convey.Convey("Given non-trade execution types", t, func() {
		payload := []byte(`{
			"channel": "executions",
			"data": [{
				"exec_type": "new",
				"cl_ord_id": "s1-abc",
				"last_qty": 1,
				"last_price": 100
			}, {
				"exec_type": "canceled",
				"cl_ord_id": "s1-abc"
			}]
		}`)
		fills, err := ParseExecutionFills(payload)

		convey.Convey("It should filter them out", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given an empty data array", t, func() {
		payload := []byte(`{"channel": "executions", "data": []}`)
		fills, err := ParseExecutionFills(payload)

		convey.Convey("It should return an empty slice", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given malformed JSON", t, func() {
		_, err := ParseExecutionFills([]byte(`{not json`))

		convey.Convey("It should return an error", func() {
			convey.So(err, convey.ShouldNotBeNil)
		})
	})

	convey.Convey("Given multiple trade rows", t, func() {
		payload := []byte(`{
			"channel": "executions",
			"data": [{
				"order_id": "O1",
				"cl_ord_id": "s1-a",
				"symbol": "ETH/EUR",
				"side": "sell",
				"exec_type": "trade",
				"exec_id": "E1",
				"last_qty": 0.5,
				"last_price": 2000
			}, {
				"order_id": "O1",
				"cl_ord_id": "s1-a",
				"symbol": "ETH/EUR",
				"side": "sell",
				"exec_type": "trade",
				"exec_id": "E2",
				"last_qty": 0.3,
				"last_price": 2001
			}]
		}`)
		fills, err := ParseExecutionFills(payload)

		convey.Convey("It should return every trade fill", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 2)
			convey.So(fills[0].ExecKey, convey.ShouldEqual, "E1")
			convey.So(fills[0].ClOrdID, convey.ShouldEqual, "s1-a")
			convey.So(fills[0].Qty, convey.ShouldEqual, 0.5)
			convey.So(fills[0].Price, convey.ShouldEqual, 2000)
			convey.So(fills[1].ExecKey, convey.ShouldEqual, "E2")
			convey.So(fills[1].Qty, convey.ShouldEqual, 0.3)
			convey.So(fills[1].Price, convey.ShouldEqual, 2001)
		})
	})
}
