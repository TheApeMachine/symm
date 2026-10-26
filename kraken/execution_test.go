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

		Convey("ToValue serializes execution into Value", func() {
			val := execution.ToValue(123456789)
			So(val, ShouldNotBeNil)
			So(val.Scope, ShouldEqual, "BTC/USD")
			So(val.Role, ShouldEqual, "execution")
			So(val.Epoch, ShouldEqual, uint64(123456789))
		})
	})
}
