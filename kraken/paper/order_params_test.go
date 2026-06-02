package paper

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestAddParamsFromAny(t *testing.T) {
	Convey("Given typed add params", t, func() {
		params := trading.AddParams{
			OrderType: trading.Limit,
			Side:      trading.Buy,
			Symbol:    "BTC/EUR",
			OrderQty:  0.1,
			ClOrdID:   "test-clord",
		}

		Convey("It should round-trip through map frames", func() {
			parsed, ok := addParamsFromAny(map[string]any{
				"order_type": "limit",
				"side":       "buy",
				"symbol":     "BTC/EUR",
				"order_qty":  0.1,
				"cl_ord_id":  "test-clord",
			})

			So(ok, ShouldBeTrue)
			So(parsed.Symbol, ShouldEqual, params.Symbol)
			So(parsed.ClOrdID, ShouldEqual, params.ClOrdID)
		})

		Convey("It should accept typed values directly", func() {
			parsed, ok := addParamsFromAny(params)

			So(ok, ShouldBeTrue)
			So(parsed.ClOrdID, ShouldEqual, "test-clord")
		})
	})
}

func TestStringList(t *testing.T) {
	Convey("Given heterogeneous order id payloads", t, func() {
		Convey("It should normalize string and slice forms", func() {
			So(stringList("abc"), ShouldResemble, []string{"abc"})
			So(stringList([]string{"a", "b"}), ShouldResemble, []string{"a", "b"})
			So(stringList([]any{"x", 1}), ShouldResemble, []string{"x"})
		})
	})
}

func TestBatchOrders(t *testing.T) {
	Convey("Given a batch add frame", t, func() {
		symbol, orders, ok := batchOrders(map[string]any{
			"symbol": "BTC/EUR",
			"orders": []trading.AddParams{
				{Symbol: "BTC/EUR", Side: trading.Buy},
				{Symbol: "BTC/EUR", Side: trading.Sell},
			},
		})

		Convey("It should extract symbol and order list", func() {
			So(ok, ShouldBeTrue)
			So(symbol, ShouldEqual, "BTC/EUR")
			So(len(orders), ShouldEqual, 2)
		})
	})
}
