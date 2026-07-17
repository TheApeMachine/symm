package types

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

/*
TestHoldingUpdateBuyRecordsFilledQty verifies CumQty/AvgPrice drive inventory.
*/
func TestHoldingUpdateBuyRecordsFilledQty(t *testing.T) {
	Convey("Given an empty holding and a buy print", t, func() {
		holding := &Holding{Qty: decimal.NewFromInt64(1)}
		entryAt := time.Unix(1, 0)
		holding.Update(&kraken.ExecutionData{
			ExecType:    "trade",
			Side:        "buy",
			Timestamp:   entryAt,
			LastQty:     0.4,
			CumQty:      0.4,
			LastPrice:   *decimal.NewFromInt64(100),
			AvgPrice:    *decimal.NewFromInt64(100),
			FeeUsdEquiv: *decimal.NewFromFloat64(0.1),
		})

		So(holding.Status, ShouldEqual, OPEN)
		So(holding.Qty.Float64(), ShouldEqual, 0.4)
		So(holding.EntryPrice.Float64(), ShouldEqual, 100)
		So(holding.ExitAt, ShouldBeNil)
	})
}

/*
TestHoldingUpdateBuyAccumulatesFees verifies multi-leg entry accounting.
*/
func TestHoldingUpdateBuyAccumulatesFees(t *testing.T) {
	Convey("Given two buy prints on one order", t, func() {
		holding := &Holding{}
		holding.Update(&kraken.ExecutionData{
			ExecType: "trade", Side: "buy", Timestamp: time.Unix(1, 0),
			LastQty: 0.4, CumQty: 0.4,
			LastPrice: *decimal.NewFromInt64(100), AvgPrice: *decimal.NewFromInt64(100),
			FeeUsdEquiv: *decimal.NewFromFloat64(0.1),
		})
		holding.Update(&kraken.ExecutionData{
			ExecType: "trade", Side: "buy", Timestamp: time.Unix(1, 0),
			LastQty: 0.6, CumQty: 1.0,
			LastPrice: *decimal.NewFromInt64(110), AvgPrice: *decimal.NewFromInt64(106),
			FeeUsdEquiv: *decimal.NewFromFloat64(0.16), OrderStatus: "filled",
		})

		So(holding.Qty.Float64(), ShouldEqual, 1.0)
		So(holding.EntryPrice.Float64(), ShouldEqual, 106)
		So(holding.EntryFee.Float64(), ShouldAlmostEqual, 0.26, 0.0000001)
	})
}

/*
TestHoldingUpdatePartialSellKeepsOpen verifies residual inventory stays tracked.
*/
func TestHoldingUpdatePartialSellKeepsOpen(t *testing.T) {
	Convey("Given a filled long and a partial exit", t, func() {
		holding := &Holding{Qty: decimal.NewFromInt64(1), Status: OPEN}
		holding.EntryAt = ptrTime(time.Unix(1, 0))
		holding.EntryPrice = decimal.NewFromInt64(100)
		holding.Update(&kraken.ExecutionData{
			ExecType: "trade", Side: "sell", Timestamp: time.Unix(2, 0),
			LastQty: 0.25, LastPrice: *decimal.NewFromInt64(101),
			FeeUsdEquiv: *decimal.NewFromFloat64(0.05),
			OrderStatus: "partially_filled",
		})

		So(holding.Status, ShouldEqual, OPEN)
		So(holding.Qty.Float64(), ShouldEqual, 0.75)
		So(holding.Closed(), ShouldBeFalse)
	})
}

/*
TestHoldingUpdateFilledSellCloses verifies OrderStatus filled closes inventory.
*/
func TestHoldingUpdateFilledSellCloses(t *testing.T) {
	Convey("Given remaining inventory and a filled sell", t, func() {
		holding := &Holding{Qty: decimal.NewFromFloat64(0.75), Status: OPEN}
		holding.EntryAt = ptrTime(time.Unix(1, 0))
		holding.EntryPrice = decimal.NewFromInt64(100)
		holding.Update(&kraken.ExecutionData{
			ExecType: "trade", Side: "sell", Timestamp: time.Unix(3, 0),
			LastQty: 0.75, LastPrice: *decimal.NewFromInt64(102),
			FeeUsdEquiv: *decimal.NewFromFloat64(0.2),
			OrderStatus: "filled",
		})

		So(holding.Status, ShouldEqual, CLOSED)
		So(holding.Closed(), ShouldBeTrue)
		So(holding.Qty.Float64(), ShouldEqual, 0)
	})
}

/*
TestHoldingUpdateCancellationPreservesEntry verifies cancel does not wipe buys.
*/
func TestHoldingUpdateCancellationPreservesEntry(t *testing.T) {
	Convey("Given entry facts and a cancel", t, func() {
		entryAt := time.Unix(1, 0)
		holding := &Holding{
			Status: OPEN, EntryAt: &entryAt,
			EntryPrice: decimal.NewFromInt64(100),
			Qty:        decimal.NewFromInt64(1),
		}
		holding.Update(&kraken.ExecutionData{ExecType: "canceled"})

		So(holding.Status, ShouldEqual, CANCELED)
		So(*holding.EntryAt, ShouldEqual, entryAt)
		So(holding.EntryPrice.Float64(), ShouldEqual, 100)
		So(holding.ExitAt, ShouldBeNil)
	})
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
