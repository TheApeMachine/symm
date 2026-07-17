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
TestHoldingUpdateBuyFirstLastQtyReplaces verifies the first LastQty-only print
replaces pre-submit size instead of accumulating against it.
*/
func TestHoldingUpdateBuyFirstLastQtyReplaces(t *testing.T) {
	Convey("Given a holding with requested size and a first LastQty-only print", t, func() {
		holding := &Holding{Qty: decimal.NewFromInt64(1)}
		holding.Update(&kraken.ExecutionData{
			ExecType:  "trade",
			Side:      "buy",
			Timestamp: time.Unix(1, 0),
			LastQty:   0.4,
			LastPrice: *decimal.NewFromInt64(100),
		})

		So(holding.Qty.Float64(), ShouldEqual, 0.4)
		So(holding.EntryPrice.Float64(), ShouldEqual, 100)
	})
}

/*
TestHoldingUpdateBuySubsequentLastQtyAccumulates verifies later LastQty-only
prints accumulate until the exchange reports CumQty.
*/
func TestHoldingUpdateBuySubsequentLastQtyAccumulates(t *testing.T) {
	Convey("Given a first LastQty-only fill and a second without CumQty", t, func() {
		holding := &Holding{Qty: decimal.NewFromInt64(1)}
		holding.Update(&kraken.ExecutionData{
			ExecType:  "trade",
			Side:      "buy",
			Timestamp: time.Unix(1, 0),
			LastQty:   0.4,
			LastPrice: *decimal.NewFromInt64(100),
		})
		holding.Update(&kraken.ExecutionData{
			ExecType:  "trade",
			Side:      "buy",
			Timestamp: time.Unix(2, 0),
			LastQty:   0.3,
			LastPrice: *decimal.NewFromInt64(110),
		})

		So(holding.Qty.Float64(), ShouldEqual, 0.7)
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
		So(holding.EntryFee.Float64(), ShouldEqual, 0.3)
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
TestHoldingProjectFillsMissingUIFields verifies restart frames stay finite.
*/
func TestHoldingProjectFillsMissingUIFields(t *testing.T) {
	Convey("Given a persisted holding with only entry accounting", t, func() {
		holding := &Holding{
			Symbol:     "BTC/USD",
			Qty:        decimal.NewFromFloat64(0.25),
			EntryPrice: decimal.NewFromFloat64(100),
			Status:     OPEN,
		}

		holding.Project()

		Convey("It should project mark and zero fees for the UI", func() {
			So(holding.Mark.Float64(), ShouldEqual, 100)
			So(holding.EntryFee.Float64(), ShouldEqual, 0)
			So(holding.ExitFee.Float64(), ShouldEqual, 0)
			So(holding.PnL.Float64(), ShouldEqual, 0)
			So(*holding.ReturnPct, ShouldEqual, 0)
		})
	})
}

/*
TestHoldingMarkToMarketUpdatesOpenPnL verifies live mark drives UI PnL.
*/
func TestHoldingMarkToMarketUpdatesOpenPnL(t *testing.T) {
	Convey("Given an open holding with a moved mark", t, func() {
		holding := &Holding{
			Symbol:     "BTC/USD",
			Qty:        decimal.NewFromFloat64(2),
			EntryPrice: decimal.NewFromFloat64(100),
			EntryFee:   decimal.NewFromFloat64(0.5),
			Mark:       decimal.NewFromFloat64(110),
			Status:     OPEN,
		}

		holding.MarkToMarket()

		So(holding.PnL.Float64(), ShouldEqual, 19.5)
		So(*holding.ReturnPct, ShouldAlmostEqual, 0.1, 0.0000001)
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
