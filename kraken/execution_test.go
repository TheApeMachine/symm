package kraken

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewExecutionDataSlice(t *testing.T) {
	Convey("Given an executions channel snapshot frame", t, func() {
		buf := []byte(`{"channel":"executions","type":"snapshot","sequence":1,"data":[{"exec_type":"snapshot","position_status":"open","symbol":"BTC/USD","last_qty":0.0001,"side":"buy","avg_price":"100000"}]}`)

		Convey("When the frame is decoded", func() {
			rows := NewExecutionDataSlice(buf)

			Convey("Then it should unwrap the channel envelope", func() {
				So(*rows, ShouldHaveLength, 1)
				So((*rows)[0].ExecType, ShouldEqual, "snapshot")
				So((*rows)[0].PositionStatus, ShouldEqual, "open")
				So((*rows)[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})

	Convey("Given a documented Kraken executions payload", t, func() {
		buf := []byte(`[{
			"order_id": "OK4GJX-KSTLS-7DZZO5",
			"order_userref": 3,
			"exec_id": "TGBB7L-HT5LX-J3BZ4A",
			"exec_type": "trade",
			"trade_id": 62887576,
			"symbol": "BTC/USD",
			"side": "sell",
			"last_qty": 0.005,
			"last_price": "26599.9",
			"liquidity_ind": "t",
			"cost": "132.9995",
			"order_type": "limit",
			"timestamp": "2023-09-22T10:33:05.709993Z",
			"order_status": "partially_filled",
			"cum_qty": 0.005,
			"cum_cost": "132.9995",
			"avg_price": "26599.9",
			"fee_usd_equiv": "0.3458",
			"fees": [{"asset": "USD", "qty": 0.3458}]
		}]`)

		rows := NewExecutionDataSlice(buf)

		Convey("Then only documented execution fields should be decoded", func() {
			So(*rows, ShouldHaveLength, 1)
			So((*rows)[0].OrderID, ShouldEqual, "OK4GJX-KSTLS-7DZZO5")
			So((*rows)[0].OrderUserRef, ShouldEqual, 3)
			So((*rows)[0].ExecID, ShouldEqual, "TGBB7L-HT5LX-J3BZ4A")
			So((*rows)[0].ExecType, ShouldEqual, "trade")
			So((*rows)[0].TradeID, ShouldEqual, 62887576)
			So((*rows)[0].Symbol, ShouldEqual, "BTC/USD")
			So((*rows)[0].LastPrice.String(), ShouldEqual, "26599.9")
			So((*rows)[0].Cost.String(), ShouldEqual, "132.9995")
			So((*rows)[0].Fees, ShouldHaveLength, 1)
			So((*rows)[0].Fees[0].Asset, ShouldEqual, "USD")
			So((*rows)[0].Fees[0].Qty, ShouldEqual, 0.3458)
			So((*rows)[0].Timestamp, ShouldResemble, time.Date(
				2023,
				time.September,
				22,
				10,
				33,
				5,
				709993000,
				time.UTC,
			))
		})
	})
}

func BenchmarkNewExecutionDataSlice(b *testing.B) {
	buf := []byte(`[{
		"order_id": "OK4GJX-KSTLS-7DZZO5",
		"exec_id": "TGBB7L-HT5LX-J3BZ4A",
		"exec_type": "trade",
		"symbol": "BTC/USD",
		"side": "sell",
		"last_qty": 0.005,
		"last_price": "26599.9",
		"cost": "132.9995",
		"timestamp": "2023-09-22T10:33:05.709993Z",
		"fees": [{"asset": "USD", "qty": 0.3458}]
	}]`)

	b.ReportAllocs()
	for b.Loop() {
		_ = NewExecutionDataSlice(buf)
	}
}
