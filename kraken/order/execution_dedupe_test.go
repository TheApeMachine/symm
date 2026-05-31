package order

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestParseExecutionFillsRejectsMissingExecKey(t *testing.T) {
	convey.Convey("Given a trade row without dedupe identifiers", t, func() {
		payload := []byte(`{
			"channel": "executions",
			"data": [{
				"order_id": "O1",
				"cl_ord_id": "s1-abc",
				"symbol": "BTC/EUR",
				"side": "buy",
				"exec_type": "trade",
				"last_qty": 0.01,
				"last_price": 50000,
				"fee": 0.2,
				"fee_ccy": "EUR"
			}]
		}`)
		fills, err := ParseExecutionFills(payload)

		convey.Convey("It should drop the fill instead of deduping fail-open", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 0)
		})
	})
}

func TestParseExecutionFillsUsesNativeFee(t *testing.T) {
	convey.Convey("Given a trade row with native fee and usd equivalent", t, func() {
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
				"last_price": 50000,
				"fee": 0.2,
				"fee_usd_equiv": 0.22,
				"fee_ccy": "EUR"
			}]
		}`)
		fills, err := ParseExecutionFills(payload)

		convey.Convey("It should prefer the native fee amount", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 1)
			convey.So(fills[0].Fee, convey.ShouldEqual, 0.2)
			convey.So(fills[0].FeeCcy, convey.ShouldEqual, "EUR")
		})
	})
}
