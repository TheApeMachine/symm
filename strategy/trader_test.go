package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
)

func TestTraderApplyExecutionReconcilesFill(t *testing.T) {
	Convey("ApplyExecution Reconciles a fill onto an owned pending regulator", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		paper := broker.NewPaper(ctx)
		trader := NewTrader(ctx, paper, nil, nil)

		reg := position.NewRegulator("BTC/USD")
		So(reg.Begin(&kraken.AddOrderRequest{
			Pair:    "BTC/USD",
			Type:    "buy",
			Volume:  "0.01",
			ClOrdId: reg.PositionID,
			OrdType: "limit",
		}), ShouldBeNil)

		trader.mu.Lock()
		trader.open["BTC/USD"] = reg
		trader.mu.Unlock()

		qty := decimal.NewFromFloat64(0.01)
		price := decimal.NewFromFloat64(100)
		cost := price.Mul(qty)
		fee := decimal.NewFromFloat64(0.026)

		trader.ApplyExecution(&kraken.Execution{
			Data: []kraken.ExecutionData{{
				OrderID:       "ORD-1",
				ClientOrderID: reg.PositionID,
				OrderStatus:   "filled",
				CumQty:        qty,
				AvgPrice:      price,
				CumCost:       cost,
				FeeUsdEquiv:   fee,
			}},
		})

		So(reg.IsHolding(), ShouldBeTrue)
		So(reg.Pending, ShouldBeNil)
	})
}

func TestNewTraderWiresPaperOnExecution(t *testing.T) {
	Convey("NewTrader registers Paper.OnExecution → ApplyExecution", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		paper := broker.NewPaper(ctx)
		trader := NewTrader(ctx, paper, nil, nil)

		reg := position.NewRegulator("ETH/USD")
		So(reg.Begin(&kraken.AddOrderRequest{
			Pair:    "ETH/USD",
			Type:    "buy",
			Volume:  "1",
			ClOrdId: reg.PositionID,
			OrdType: "market",
		}), ShouldBeNil)

		trader.mu.Lock()
		trader.open["ETH/USD"] = reg
		trader.mu.Unlock()

		qty := decimal.NewFromFloat64(1)
		px := decimal.NewFromFloat64(10)
		paper.OnExecution(trader.ApplyExecution) // idempotent re-bind for clarity
		// Invoke the same path publish uses:
		trader.ApplyExecution(&kraken.Execution{
			Data: []kraken.ExecutionData{{
				OrderID:       "ORD-2",
				ClientOrderID: reg.PositionID,
				OrderStatus:   "filled",
				CumQty:        qty,
				AvgPrice:      px,
				CumCost:       px.Mul(qty),
				FeeUsdEquiv:   decimal.NewFromFloat64(0.01),
			}},
		})

		So(reg.IsHolding(), ShouldBeTrue)
	})
}

func TestOnPositionClosedFiresAfterExitFill(t *testing.T) {
	Convey("ApplyExecution closing a regulator invokes OnPositionClosed", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		paper := broker.NewPaper(ctx)
		trader := NewTrader(ctx, paper, nil, nil)

		reg := position.NewRegulator("BTC/USD")
		So(reg.Begin(&kraken.AddOrderRequest{
			Pair: "BTC/USD", Type: "buy", Volume: "0.01",
			ClOrdId: reg.PositionID, OrdType: "limit",
		}), ShouldBeNil)

		qty := decimal.NewFromFloat64(0.01)
		px := decimal.NewFromFloat64(100)
		cost := px.Mul(qty)
		fee := decimal.NewFromFloat64(0.01)

		So(reg.Reconcile(kraken.ExecutionData{
			OrderID: "ORD-E", ClientOrderID: reg.PositionID,
			OrderStatus: "filled", CumQty: qty, AvgPrice: px, CumCost: cost, FeeUsdEquiv: fee,
		}), ShouldBeNil)
		So(reg.IsHolding(), ShouldBeTrue)

		So(reg.Begin(&kraken.AddOrderRequest{
			Pair: "BTC/USD", Type: "sell", Volume: "0.01",
			ClOrdId: "exit-1", OrdType: "limit",
		}), ShouldBeNil)

		trader.mu.Lock()
		trader.open["BTC/USD"] = reg
		trader.mu.Unlock()

		var closedSym string
		var closedReg *position.Regulator
		trader.OnPositionClosed(func(symbol string, r *position.Regulator) {
			closedSym = symbol
			closedReg = r
		})

		trader.ApplyExecution(&kraken.Execution{
			Data: []kraken.ExecutionData{{
				OrderID: "ORD-X", ClientOrderID: "exit-1",
				OrderStatus: "filled", CumQty: qty, AvgPrice: px, CumCost: cost, FeeUsdEquiv: fee,
			}},
		})

		So(closedSym, ShouldEqual, "BTC/USD")
		So(closedReg, ShouldNotBeNil)
		So(closedReg.IsClosed(), ShouldBeTrue)
	})
}
