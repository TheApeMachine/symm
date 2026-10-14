package kraken_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
)

func TestNewExecutionFromMap(t *testing.T) {
	Convey("Given a paper execution carrying its client order identifier", t, func() {
		execution := kraken.NewExecutionFromMap(datura.Map[any]{
			"cl_ord_id": "exit-order-id",
			"pair":      "SLAY/USD",
			"side":      "sell",
		})

		Convey("The normalized execution should preserve order correlation", func() {
			So(execution.Data, ShouldHaveLength, 1)
			So(execution.Data[0].ClientOrderID, ShouldEqual, "exit-order-id")
		})
	})
}

func TestExecutionMeasurementRoundTrip(t *testing.T) {
	Convey("Given a valid kraken.Execution with trade fill data", t, func() {
		price, _ := decimal.NewFromString("50000.5")
		qty, _ := decimal.NewFromString("0.1")
		cost, _ := decimal.NewFromString("5000.05")
		fee, _ := decimal.NewFromString("12.50")

		execData := kraken.ExecutionData{
			Symbol:        "BTC/USD",
			OrderID:       "ord-999",
			ClientOrderID: "cl-ord-888",
			ExecID:        "exec-777",
			ExecType:      "trade",
			Side:          "buy",
			OrderType:     "limit",
			OrderStatus:   "filled",
			LastPrice:     price,
			LastQty:       qty,
			Cost:          cost,
			CumQty:        qty,
			CumCost:       cost,
			AvgPrice:      price,
			FeeUsdEquiv:   fee,
		}

		execution := &kraken.Execution{
			Channel: "executions",
			Type:    "update",
			Data:    []kraken.ExecutionData{execData},
		}

		Convey("ToMeasurement serializes execution into structured metrics without raw shortcuts", func() {
			meas := execution.ToMeasurement(123456789)
			So(meas, ShouldNotBeNil)
			So(meas.Label, ShouldEqual, "BTC/USD")
			So(meas.Source, ShouldEqual, "executions")
			So(meas.Raw(), ShouldBeNil)

			Convey("and NewExecutionFromMeasurement reconstructs execution with exact values", func() {
				restored, err := kraken.NewExecutionFromMeasurement(meas)
				So(err, ShouldBeNil)
				So(restored, ShouldNotBeNil)
				So(len(restored.Data), ShouldEqual, 1)

				item := restored.Data[0]
				So(item.Symbol, ShouldEqual, "BTC/USD")
				So(item.OrderID, ShouldEqual, "ord-999")
				So(item.ClientOrderID, ShouldEqual, "cl-ord-888")
				So(item.ExecID, ShouldEqual, "exec-777")
				So(item.ExecType, ShouldEqual, "trade")
				So(item.Side, ShouldEqual, "buy")
				So(item.OrderType, ShouldEqual, "limit")
				So(item.OrderStatus, ShouldEqual, "filled")
				So(item.LastPrice.String(), ShouldEqual, "50000.5")
				So(item.LastQty.String(), ShouldEqual, "0.1")
				So(item.Cost.String(), ShouldEqual, "5000.05")
				So(item.CumQty.String(), ShouldEqual, "0.1")
				So(item.CumCost.String(), ShouldEqual, "5000.05")
				So(item.AvgPrice.String(), ShouldEqual, "50000.5")
				So(item.FeeUsdEquiv.String(), ShouldEqual, "12.50")
			})
		})
	})
}

