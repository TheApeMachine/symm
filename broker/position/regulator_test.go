package position

import (
	venue "github.com/theapemachine/symm/tests/venue"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
closeFillPosition builds a minimal open lot whose realized exit economics can be
asserted without a live exchange or database. sellable is the complete
filled inventory; entryPrice and entryFee are the authoritative entry basis.
*/
func closeFillPosition(sellable string, entryPrice string, entryFee string) *Regulator {
	quantity, _ := decimal.NewFromString(sellable)
	price, _ := decimal.NewFromString(entryPrice)
	fee, _ := decimal.NewFromString(entryFee)
	cost := price.SetScale(decimal.DefaultScale).Mul(quantity)

	holding := &types.Holding{
		Symbol:      "SHAPE/USD",
		Status:      types.OPEN,
		Qty:         quantity,
		SellableQty: quantity,
		Basis:       cost,
		EntryCost:   cost,
		EntryPrice:  price,
		EntryFee:    fee,
		EntryFees:   fee,
		EntryQty:    quantity,
		ExitCost:    decimal.NewFromInt64(0),
		ExitFees:    decimal.NewFromInt64(0),
		ExitQty:     decimal.NewFromInt64(0),
		RealizedPnL: decimal.NewFromInt64(0),
	}

	instrument := broker.NewInstrumentWithQuote("USD")
	priceSvc := broker.NewPrice(nil, instrument)

	regulator := &Regulator{
		Holding: holding,
		price:   priceSvc,
		pending: &spot.AddOrderRequest{
			ClOrdId: "shape-exit",
			Pair:    "SHAPE/USD",
			Type:    "sell",
			Volume:  sellable,
		},
	}
	regulator.Guardian = NewGuardian(regulator)
	return regulator
}

func TestPositionWire(t *testing.T) {
	Convey("Given an open position", t, func() {
		position := closeFillPosition("100000", "0.000490", "0.04")

		Convey("the wire snapshot includes status and holding facts", func() {
			encoded := position.Wire()

			So(encoded, ShouldNotBeNil)
			So(encoded.Status, ShouldEqual, "open")
			So(encoded.Holding, ShouldNotBeNil)
			So(encoded.Holding.Symbol, ShouldEqual, "SHAPE/USD")
		})
	})
}

/*
executionFixture builds a Kraken ExecutionData with a last individual fill whose
LastPrice is materially different from the whole-order AvgPrice, so a
close-fill path that marks the exit by the last fill would diverge from the
authoritative whole-order realized VWAP.
*/
func executionFixture(
	lastPrice string,
	avgPrice string,
	cumQty string,
	cumCost string,
	feeUsd string,
) kraken.ExecutionData {
	return kraken.ExecutionData{
		ExecID:        "exit-1",
		ClientOrderID: "shape-exit",
		Side:          "sell",
		LastQty:       venue.Decimal(cumQty),
		LastPrice:     venue.Decimal(lastPrice),
		CumQty:        venue.Decimal(cumQty),
		CumCost:       venue.Decimal(cumCost),
		AvgPrice:      venue.Decimal(avgPrice),
		FeeUsdEquiv:   venue.Decimal(feeUsd),
		Timestamp:     time.Now(),
		OrderStatus:   "filled",
	}
}

func TestCloseFillWholeOrderVWAP(t *testing.T) {
	Convey("Given a multi-fill exit where the last fill differs from the whole-order average", t, func() {
		position := closeFillPosition("100000", "0.000490", "0.04")
		execution := executionFixture("0.000520", "0.000506", "100000", "50.6", "0.40")
		sellable := venue.Decimal("100000")
		avgPrice := venue.Decimal("0.000506")

		err := position.Apply(execution)
		So(err, ShouldBeNil)

		Convey("the exit price is the whole-order realized VWAP, not the last fill", func() {
			So(position.Holding.ExitVWAP.Cmp(avgPrice), ShouldEqual, 0)
			So(position.Holding.ExitPrice.Cmp(avgPrice), ShouldEqual, 0)
			So(position.Holding.ExitQty.Cmp(sellable), ShouldEqual, 0)
		})

		Convey("PnL and ReturnPct reconcile from the same realized economics", func() {
			// entry basis 0.000490 × 100000 = 49, + entry fee 0.04 = 49.04
			// exit proceeds 50.6 − exit fee 0.40 = 50.2
			// realized PnL = 50.2 − 49.04 = 1.16
			So(position.Holding.PnL.Float64(), ShouldAlmostEqual, 1.16, 1e-12)
			So(position.Holding.RealizedPnL.Float64(), ShouldAlmostEqual, 1.16, 1e-12)

			expectedReturn := 1.16 / 49.04 * 100
			So(position.Holding.ReturnPct, ShouldAlmostEqual, expectedReturn, 1e-9)

			realizedPct := position.Holding.RealizedReturn.Float64() * 100
			So(realizedPct-expectedReturn < 1e-8 && expectedReturn-realizedPct < 1e-8, ShouldBeTrue)
		})

		Convey("exit fees are the exchange's authoritative total", func() {
			So(position.Holding.ExitFees.Cmp(venue.Decimal("0.40")), ShouldEqual, 0)
			So(position.Holding.ExitFee.Cmp(venue.Decimal("0.40")), ShouldEqual, 0)
		})
	})
}

func TestCloseFillAvgPriceFallback(t *testing.T) {
	Convey("Given an exit execution without an explicit AvgPrice", t, func() {
		position := closeFillPosition("100000", "0.000490", "0.04")
		execution := executionFixture("0.000520", "0", "100000", "50.6", "0.40")
		execution.AvgPrice = nil

		Convey("the exit VWAP falls back to the cumulative CumCost/CumQty equivalent", func() {
			err := position.Apply(execution)

			So(err, ShouldBeNil)
			So(position.Holding.ExitVWAP.Cmp(venue.Decimal("0.000506")), ShouldEqual, 0)
		})
	})
}

func TestPartialFillExitAccumulatesWholeOrder(t *testing.T) {
	Convey("Given a multi-fill exit with a duplicate terminal fill", t, func() {
		position := closeFillPosition("100000", "0.000490", "0.04")

		partialOne := executionFixture("0.000510", "0.000510", "40000", "20.4", "0.10")
		partialOne.ExecID = "exit-partial-1"
		partialOne.OrderStatus = "partially_filled"

		partialTwo := executionFixture("0.000505", "0.000505", "60000", "30.3", "0.15")
		partialTwo.ExecID = "exit-partial-2"
		partialTwo.OrderStatus = "partially_filled"

		terminal := executionFixture("0.000502", "0.000504", "100000", "50.4", "0.25")
		terminal.ExecID = "exit-terminal"
		terminal.OrderStatus = "filled"

		dupe := terminal
		dupe.ExecID = "exit-terminal"

		Convey("the terminal fill cumulative whole-order VWAP is the exit price, and duplicates do not double-count", func() {
			err := position.Apply(partialOne)
			So(err, ShouldBeNil)

			err = position.Apply(partialTwo)
			So(err, ShouldBeNil)

			err = position.Apply(terminal)
			So(err, ShouldBeNil)

			// Terminal fill cleared pending order, so duplicate does nothing
			err = position.Apply(dupe)
			So(err, ShouldBeNil)

			So(position.Holding.ExitVWAP.Cmp(venue.Decimal("0.000504")), ShouldEqual, 0)
			So(position.Holding.ExitQty.Cmp(venue.Decimal("100000")), ShouldEqual, 0)
			So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)

			// 0.000490 × 100000 = 49, + 0.04 fee = 49.04 basis.
			// 50.4 − 0.25 = 50.15 proceeds. PnL = 1.11.
			So(position.Holding.PnL.Float64(), ShouldAlmostEqual, 1.11, 1e-9)
			So(position.Status(), ShouldEqual, types.CLOSED)
		})
	})
}

func TestPositionOnExecutionTerminalPartialEntry(t *testing.T) {
	Convey("Given an entry that partially fills before Kraken cancels its remainder", t, func() {
		instrument := broker.NewInstrumentWithQuote("USD")
		priceSvc := broker.NewPrice(nil, instrument)
		position := &Regulator{
			Holding: types.NewHolding("TEST/USD"),
			price:   priceSvc,
			pending: &spot.AddOrderRequest{
				ClOrdId: "entry-order",
				Pair:    "TEST/USD",
				Type:    "buy",
				Volume:  "100",
			},
		}
		position.Holding.Status = types.PENDING

		execution := kraken.ExecutionData{
			OrderID:       "venue-entry",
			ClientOrderID: "entry-order",
			ExecID:        "entry-terminal-partial",
			ExecType:      "canceled",
			Symbol:        "TEST/USD",
			Side:          "buy",
			LastQty:       venue.Decimal("40"),
			LastPrice:     venue.Decimal("2.00"),
			CumQty:        venue.Decimal("40"),
			CumCost:       venue.Decimal("80.00"),
			AvgPrice:      venue.Decimal("2.00"),
			FeeUsdEquiv:   venue.Decimal("0.20"),
			Timestamp:     time.Now(),
			OrderStatus:   "canceled",
		}

		Convey("the filled inventory remains open and owned", func() {
			err := position.Apply(execution)

			So(err, ShouldBeNil)
			So(position.Status(), ShouldEqual, types.OPEN)
			So(position.Holding.Status, ShouldEqual, types.OPEN)
			So(position.Holding.Qty.Cmp(venue.Decimal("40")), ShouldEqual, 0)
			So(position.Holding.SellableQty.Cmp(venue.Decimal("40")), ShouldEqual, 0)
			So(position.Holding.EntryPrice.Cmp(venue.Decimal("2.00")), ShouldEqual, 0)
			So(position.Holding.EntryFee.Cmp(venue.Decimal("0.20")), ShouldEqual, 0)
		})
	})
}

func TestPositionOnExecutionActionCorrelation(t *testing.T) {
	Convey("Given a lot with distinct reduction and exit orders", t, func() {
		position := closeFillPosition("10", "100", "0")
		var terminalIDs []string
		position.record = func(execution kraken.ExecutionData) error {
			if execution.OrderStatus == "filled" {
				terminalIDs = append(terminalIDs, execution.ClientOrderID)
			}
			return nil
		}

		// First: a reduction of 2
		position.pending = &spot.AddOrderRequest{
			ClOrdId: "reduce-order",
			Pair:    "SHAPE/USD",
			Type:    "sell",
			Volume:  "2",
		}
		reduceExec := executionFixture("120", "120", "2", "240", "0")
		reduceExec.ClientOrderID = "reduce-order"
		reduceExec.OrderStatus = "filled"

		err := position.Apply(reduceExec)
		So(err, ShouldBeNil)
		So(position.Holding.Qty.Cmp(venue.Decimal("8")), ShouldEqual, 0)
		So(position.Pending(), ShouldBeNil)

		// Second: an exit of remaining 8
		position.pending = &spot.AddOrderRequest{
			ClOrdId: "exit-order",
			Pair:    "SHAPE/USD",
			Type:    "sell",
			Volume:  "8",
		}
		exitExec := executionFixture("80", "80", "8", "640", "0")
		exitExec.ClientOrderID = "exit-order"
		exitExec.OrderStatus = "filled"

		err = position.Apply(exitExec)
		So(err, ShouldBeNil)
		So(position.Holding.Qty.Sign(), ShouldEqual, 0)
		So(position.Status(), ShouldEqual, types.CLOSED)
		So(terminalIDs, ShouldResemble, []string{"reduce-order", "exit-order"})
	})
}

func BenchmarkPositionApply(b *testing.B) {
	position := closeFillPosition("10", "100", "0")
	execution := kraken.ExecutionData{
		ClientOrderID: "shape-exit",
		Side:          "sell",
		CumQty:        venue.Decimal("10"),
		CumCost:       venue.Decimal("1000"),
		FeeUsdEquiv:   venue.Decimal("0"),
		OrderStatus:   "filled",
	}
	b.ReportAllocs()
	for b.Loop() {
		position.pending = &spot.AddOrderRequest{ClOrdId: "shape-exit"}
		_ = position.Apply(execution)
	}
}

func BenchmarkPositionWire(b *testing.B) {
	position := closeFillPosition("100000", "0.000490", "0.04")
	b.ReportAllocs()
	for b.Loop() {
		_ = position.Wire()
	}
}
