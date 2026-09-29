package position

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestRegulator(t *testing.T) {
	Convey("Given a new Regulator for BTC/USD", t, func() {
		reg := NewRegulator("BTC/USD")

		So(reg, ShouldNotBeNil)
		So(reg.Symbol, ShouldEqual, "BTC/USD")
		So(reg.Status(), ShouldEqual, "closed")
		So(reg.IsHolding(), ShouldBeFalse)
		So(reg.IsClosed(), ShouldBeTrue)

		Convey("When an entry order is initiated with Begin", func() {
			entryReq := &spot.AddOrderRequest{
				Pair:    "BTC/USD",
				Type:    "buy",
				Volume:  "0.01",
				Price:   "50000",
				ClOrdId: reg.PositionID,
			}

			err := reg.Begin(entryReq)
			So(err, ShouldBeNil)
			So(reg.Status(), ShouldEqual, "entry_pending")
			So(reg.IsHolding(), ShouldBeFalse)
			So(reg.IsClosed(), ShouldBeFalse)
			So(reg.Identifies("", reg.PositionID), ShouldBeTrue)

			duplicateReq := &spot.AddOrderRequest{
				Pair:    "BTC/USD",
				Type:    "buy",
				Volume:  "0.01",
				ClOrdId: "other",
			}
			So(reg.Begin(duplicateReq), ShouldNotBeNil)
		})
	})

	Convey("Given an entry order with partial fill", t, func() {
		reg := NewRegulator("BTC/USD")
		entryReq := &spot.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			Volume:  "0.01",
			Price:   "50000",
			ClOrdId: reg.PositionID,
		}
		_ = reg.Begin(entryReq)

		report := kraken.ExecutionData{
			OrderID:       "ORD-1234",
			ClientOrderID: reg.PositionID,
			Symbol:        "BTC/USD",
			OrderStatus:   "open",
			CumQty:        decimal.NewFromFloat64(0.005),
			CumCost:       decimal.NewFromFloat64(250.0),
			FeeUsdEquiv:   decimal.NewFromFloat64(0.50),
		}

		err := reg.Reconcile(report)
		So(err, ShouldBeNil)
		So(reg.Status(), ShouldEqual, "entry_pending")
		So(reg.IsHolding(), ShouldBeTrue)
		So(reg.Volume().Float64(), ShouldEqual, 0.005)
		So(reg.Price().Float64(), ShouldEqual, 50000.0)
		So(reg.Fee().Float64(), ShouldEqual, 0.50)
		So(reg.Identifies("ORD-1234", ""), ShouldBeTrue)

		Convey("When the remaining volume fills and order status is filled", func() {
			finalReport := kraken.ExecutionData{
				OrderID:       "ORD-1234",
				ClientOrderID: reg.PositionID,
				Symbol:        "BTC/USD",
				OrderStatus:   "filled",
				CumQty:        decimal.NewFromFloat64(0.01),
				CumCost:       decimal.NewFromFloat64(500.0),
				FeeUsdEquiv:   decimal.NewFromFloat64(1.00),
			}

			err := reg.Reconcile(finalReport)
			So(err, ShouldBeNil)
			So(reg.Status(), ShouldEqual, "open")
			So(reg.IsHolding(), ShouldBeTrue)
			So(reg.Volume().Float64(), ShouldEqual, 0.01)
			So(reg.Basis.Float64(), ShouldEqual, 500.0)
			So(reg.Fee().Float64(), ShouldEqual, 1.00)
		})
	})

	Convey("Given an open position ready for exit", t, func() {
		reg := NewRegulator("BTC/USD")
		entryReq := &spot.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			Volume:  "0.01",
			Price:   "50000",
			ClOrdId: reg.PositionID,
		}
		_ = reg.Begin(entryReq)

		_ = reg.Reconcile(kraken.ExecutionData{
			OrderID:       "ORD-1234",
			ClientOrderID: reg.PositionID,
			Symbol:        "BTC/USD",
			OrderStatus:   "filled",
			CumQty:        decimal.NewFromFloat64(0.01),
			CumCost:       decimal.NewFromFloat64(500.0),
			FeeUsdEquiv:   decimal.NewFromFloat64(1.00),
		})

		So(reg.Status(), ShouldEqual, "open")
		So(reg.IsHolding(), ShouldBeTrue)

		Convey("When initiating an exit order", func() {
			exitReq := &spot.AddOrderRequest{
				Pair:    "BTC/USD",
				Type:    "sell",
				Volume:  reg.Volume().String(),
				ClOrdId: "EXIT-001",
			}

			err := reg.Begin(exitReq)
			So(err, ShouldBeNil)
			So(reg.Status(), ShouldEqual, "exit_pending")
			So(reg.IsHolding(), ShouldBeTrue)

			Convey("When the exit order fills at a higher price", func() {
				exitFill := kraken.ExecutionData{
					OrderID:       "EXIT-ORD-999",
					ClientOrderID: "EXIT-001",
					Symbol:        "BTC/USD",
					OrderStatus:   "filled",
					CumQty:        decimal.NewFromFloat64(0.01),
					CumCost:       decimal.NewFromFloat64(550.0),
					FeeUsdEquiv:   decimal.NewFromFloat64(1.10),
				}

				err := reg.Reconcile(exitFill)
				So(err, ShouldBeNil)
				So(reg.Status(), ShouldEqual, "closed")
				So(reg.IsHolding(), ShouldBeFalse)
				So(reg.IsClosed(), ShouldBeTrue)
				So(reg.Volume().Sign(), ShouldEqual, 0)
				// Realized PnL: 550 (cost) - 1.10 (sell fee) - 500 (basis) - 1.00 (entry fee) = 47.90
				So(reg.Realized.Float64(), ShouldAlmostEqual, 47.90, 1e-6)
			})

			Convey("When the exit order is canceled before filling", func() {
				cancelReport := kraken.ExecutionData{
					OrderID:       "EXIT-ORD-999",
					ClientOrderID: "EXIT-001",
					Symbol:        "BTC/USD",
					OrderStatus:   "canceled",
				}

				err := reg.Reconcile(cancelReport)
				So(err, ShouldBeNil)
				So(reg.Status(), ShouldEqual, "open")
				So(reg.IsHolding(), ShouldBeTrue)
				So(reg.Volume().Float64(), ShouldEqual, 0.01)
			})
		})
	})

	Convey("Given a pending entry order that is rejected", t, func() {
		reg := NewRegulator("BTC/USD")
		entryReq := &spot.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			Volume:  "0.01",
			Price:   "50000",
			ClOrdId: reg.PositionID,
		}
		_ = reg.Begin(entryReq)

		rejectReport := kraken.ExecutionData{
			OrderID:       "ORD-REJECT",
			ClientOrderID: reg.PositionID,
			Symbol:        "BTC/USD",
			OrderStatus:   "rejected",
		}

		err := reg.Reconcile(rejectReport)
		So(err, ShouldBeNil)
		So(reg.Status(), ShouldEqual, "closed")
		So(reg.IsHolding(), ShouldBeFalse)
		So(reg.IsClosed(), ShouldBeTrue)
	})
}
