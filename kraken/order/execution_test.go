package order

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseExecutionEvents(t *testing.T) {
	Convey("Given an executions frame", t, func() {
		payload := []byte(`{
			"channel": "executions",
			"data": [{
				"order_id": "ORDER-1",
				"cl_ord_id": "cl-1",
				"symbol": "BTC/EUR",
				"side": "buy",
				"order_type": "market",
				"exec_type": "trade",
				"exec_id": "uuid-1",
				"trade_id": 99,
				"timestamp": "2026-01-01T00:00:00Z",
				"last_qty": 0.002,
				"last_price": 95000,
				"fee_usd_equiv": 0.5,
				"fee_ccy": "EUR"
			}]
		}`)

		events, err := ParseExecutionEvents(payload)

		Convey("It should parse every execution row", func() {
			So(err, ShouldBeNil)
			So(len(events), ShouldEqual, 1)
			So(events[0].ExecID, ShouldEqual, "uuid-1")
			So(events[0].TradeID, ShouldEqual, 99)
			So(events[0].Fee, ShouldAlmostEqual, 0.5, 1e-12)
		})
	})

	Convey("Given a non-executions frame", t, func() {
		_, err := ParseExecutionEvents([]byte(`{"channel":"ticker"}`))
		Convey("It should reject the frame", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestParseExecutionFillsExecKey(t *testing.T) {
	Convey("Given fill key fallbacks", t, func() {
		Convey("It should prefer exec_id", func() {
			key := execKeyFor(ExecutionEvent{ExecID: "uuid", OrderID: "o", TradeID: 1})
			So(key, ShouldEqual, "uuid")
		})

		Convey("It should fall back to order_id:trade_id", func() {
			key := execKeyFor(ExecutionEvent{OrderID: "o", TradeID: 7})
			So(key, ShouldEqual, "o:7")
		})

		Convey("It should fall back to order_id:timestamp", func() {
			key := execKeyFor(ExecutionEvent{OrderID: "o", Timestamp: "ts"})
			So(key, ShouldEqual, "o:ts")
		})
	})

	Convey("Given non-trade execution rows", t, func() {
		payload := []byte(`{
			"channel": "executions",
			"data": [{
				"order_id": "ORDER-1",
				"exec_type": "new",
				"last_qty": 1,
				"last_price": 100
			}]
		}`)

		fills, err := ParseExecutionFills(payload)

		Convey("It should skip non-trade events", func() {
			So(err, ShouldBeNil)
			So(len(fills), ShouldEqual, 0)
		})
	})
}

func TestFindOTOStopOrderIDFilters(t *testing.T) {
	Convey("Given mixed execution events", t, func() {
		events := []ExecutionEvent{
			{OrderID: "PARENT", ExecType: "trade"},
			{OrderID: "STOP-1", Symbol: "BTC/EUR", Side: "sell", OrderType: "stop-loss", ExecType: "new", OrdRefID: "PARENT"},
			{OrderID: "STOP-2", Symbol: "ETH/EUR", Side: "sell", OrderType: "stop-loss", ExecType: "new", OrdRefID: "PARENT"},
			{OrderID: "STOP-3", Symbol: "BTC/EUR", Side: "buy", OrderType: "stop-loss", ExecType: "new", OrdRefID: "PARENT"},
		}

		Convey("It should match only the sell stop for the parent symbol", func() {
			So(FindOTOStopOrderID(events, "PARENT", "BTC/EUR"), ShouldEqual, "STOP-1")
			So(FindOTOStopOrderID(events, "PARENT", "ETH/EUR"), ShouldEqual, "STOP-2")
			So(FindOTOStopOrderID(events, "MISSING", "BTC/EUR"), ShouldEqual, "")
		})
	})
}

func BenchmarkParseExecutionFills(b *testing.B) {
	payload := []byte(`{
		"channel": "executions",
		"data": [{
			"order_id": "ORDER-1",
			"symbol": "BTC/EUR",
			"side": "buy",
			"exec_type": "trade",
			"exec_id": "uuid-1",
			"last_qty": 0.001,
			"last_price": 95000
		}]
	}`)

	for b.Loop() {
		_, _ = ParseExecutionFills(payload)
	}
}
