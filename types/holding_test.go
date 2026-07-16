package types

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

/*
TestHoldingUpdate verifies entry, exit, and cancellation executions preserve
the distinction between present facts and absent pointer fields.
*/
func TestHoldingUpdate(t *testing.T) {
	Convey("Given an empty holding", t, func() {
		holding := &Holding{}
		entryAt := time.Unix(1, 0)
		holding.Update(&kraken.ExecutionData{
			ExecType:    "trade",
			Side:        "buy",
			Timestamp:   entryAt,
			LastPrice:   *decimal.NewFromInt64(100),
			FeeUsdEquiv: *decimal.NewFromFloat64(0.26),
		})

		Convey("A buy should set entry facts without manufacturing exit facts", func() {
			So(holding.Status, ShouldEqual, OPEN)
			So(*holding.EntryAt, ShouldEqual, entryAt)
			So(holding.EntryPrice.Float64(), ShouldEqual, 100)
			So(holding.ExitAt, ShouldBeNil)
			So(holding.ExitPrice, ShouldBeNil)
		})

		Convey("A cancellation should preserve the recorded entry facts", func() {
			holding.Update(&kraken.ExecutionData{ExecType: "canceled"})

			So(holding.Status, ShouldEqual, CANCELED)
			So(*holding.EntryAt, ShouldEqual, entryAt)
			So(holding.EntryPrice.Float64(), ShouldEqual, 100)
			So(holding.ExitAt, ShouldBeNil)
		})

		Convey("A sell should record exit facts without replacing entry facts", func() {
			exitAt := time.Unix(2, 0)
			holding.Update(&kraken.ExecutionData{
				ExecType:    "trade",
				Side:        "sell",
				Timestamp:   exitAt,
				LastPrice:   *decimal.NewFromInt64(101),
				FeeUsdEquiv: *decimal.NewFromFloat64(0.2626),
			})

			So(*holding.EntryAt, ShouldEqual, entryAt)
			So(holding.EntryPrice.Float64(), ShouldEqual, 100)
			So(*holding.ExitAt, ShouldEqual, exitAt)
			So(holding.ExitPrice.Float64(), ShouldEqual, 101)
		})
	})
}
