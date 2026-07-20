package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

/*
TestNewExecutionFromMap verifies that the paper adapter retains the exact fill
identity, symbol, quantity, cost, and exchange-reported fee used by accounting.
*/
func TestNewExecutionFromMap(t *testing.T) {
	Convey("Given one completed paper execution", t, func() {
		execution := NewExecutionFromMap(datura.Map[any]{
			"id": "PAPER-00042", "order_id": "PAPER-00041",
			"pair": "BTC/USD", "side": "sell", "volume": 0.00299963,
			"price": 64838.8, "cost": 194.492409644, "fee": 0.5056802650744,
			"status": "filled", "time": "2026-07-14T21:42:10Z",
		})

		Convey("Then accounting receives the source fill unchanged", func() {
			So(execution.Data, ShouldHaveLength, 1)
			fill := execution.Data[0]
			So(fill.ExecID, ShouldEqual, "PAPER-00042")
			So(fill.Symbol, ShouldEqual, "BTC/USD")
			So(fill.LastQty.String(), ShouldEqual, "0.00299963")
			So(fill.Cost.Float64(), ShouldEqual, 194.492409644)
			So(fill.FeeUsdEquiv.Float64(), ShouldEqual, 0.5056802650744)
		})
	})
}

/*
BenchmarkNewExecutionFromMap measures conversion of the paper client's real fill.
*/
func BenchmarkNewExecutionFromMap(b *testing.B) {
	model := datura.Map[any]{
		"id": "PAPER-00042", "order_id": "PAPER-00041",
		"pair": "BTC/USD", "side": "sell", "volume": 0.00299963,
		"price": 64838.8, "cost": 194.492409644, "fee": 0.5056802650744,
		"status": "filled", "time": "2026-07-14T21:42:10Z",
	}

	for b.Loop() {
		_ = NewExecutionFromMap(model)
	}
}
