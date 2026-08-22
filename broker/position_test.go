package broker

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/tests/mock"
	"github.com/theapemachine/symm/types"
)

func TestPositionOnTicker(t *testing.T) {
	Convey("Given a position whose entry has not filled", t, func() {
		price, _ := newPriceSurface(t, "SIM/USD")
		forecast := &learning.RLSOutput{Value: -0.01, Ready: true, Scale: 0.01, DegreesOfFreedom: 1}

		zeroRate := decimal.NewFromInt64(0)
		stoploss, err := types.NewStoploss(
			context.Background(),
			"SIM/USD",
			decimal.NewFromFloat64(100.02),
			decimal.NewFromFloat64(100),
			forecast,
			nil,
			decimal.NewFromFloat64(0.01),
			zeroRate,
			zeroRate,
		)
		So(err, ShouldBeNil)

		position := &Position{
			price: price,
			Holding: &types.Holding{
				Qty:      decimal.NewFromInt64(0),
				Mark:     decimal.NewFromFloat64(100),
				Stoploss: stoploss,
			},
		}
		position.setStatus(types.PENDING)

		Convey("It should remember the mark without triggering an unowned lot", func() {
			position.onTicker(kraken.TickerData{
				Bid: decimal.NewFromFloat64(98),
			})

			So(position.Holding.Mark.Cmp(decimal.NewFromFloat64(98)), ShouldEqual, 0)
			So(stoploss.Mark.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)
			So(stoploss.Status, ShouldEqual, types.ARMED)
		})
	})

	Convey("Given a filled profitable lot that stops making new highs", t, func() {
		price, api := newPriceSurface(t, "SIM/USD")
		price.Update(&kraken.TickerData{
			Symbol: "SIM/USD",
			Bid:    decimal.NewFromFloat64(103),
			Ask:    decimal.NewFromFloat64(103.02),
		})
		stoploss := newBrokerStoploss(t)
		stoploss.Update(stoploss.ArmAt)
		position := &Position{
			api:   api,
			price: price,
			ui:    newPositionUI(),
			pair: kraken.InstrumentPair{
				Symbol: "SIM/USD",
				Base:   "SIM",
				Quote:  "USD",
			},
			EntryOrder: &spot.AddOrderRequest{ClOrdId: "entry"},
			Holding: &types.Holding{
				Qty:         decimal.NewFromInt64(1),
				SellableQty: decimal.NewFromInt64(1),
				EntryPrice:  decimal.NewFromFloat64(100),
				EntryFee:    decimal.NewFromFloat64(0.25),
				Mark:        stoploss.Mark,
				Stoploss:    stoploss,
			},
		}
		position.setStatus(types.OPEN)

		Convey("It should hold a flat repeat and submit the exit when protection is breached", func() {
			// A surge-armed lot no longer exits on the first mark that fails
			// to advance: breathing under a peak is ordinary. The exit is
			// owned by a boundary — here the locked profit floor.
			position.onTicker(kraken.TickerData{
				Symbol: "SIM/USD",
				Bid:    stoploss.Mark,
			})

			So(stoploss.Status, ShouldEqual, types.ARMED)

			position.onTicker(kraken.TickerData{
				Symbol: "SIM/USD",
				Bid:    decimal.NewFromFloat64(99.99),
			})

			So(stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(position.ExitOrder, ShouldNotBeNil)
			So(position.status(), ShouldEqual, types.PENDING)
		})
	})
}

func TestPositionMarkFeedback(t *testing.T) {
	Convey("Given a live marked position with stop geometry", t, func() {
		stoploss := newBrokerStoploss(t)
		stoploss.Peak = decimal.NewFromFloat64(105)
		stoploss.Floor = decimal.NewFromFloat64(98)
		stoploss.SurgeArmed = true
		position := &Position{
			pair: kraken.InstrumentPair{Symbol: "SIM/USD"},
			Holding: &types.Holding{
				Symbol:    "SIM/USD",
				Qty:       decimal.NewFromInt64(1),
				Mark:      decimal.NewFromFloat64(100),
				PnL:       decimal.NewFromFloat64(1.25),
				ReturnPct: 1.25,
				Stoploss:  stoploss,
			},
		}

		feedback := position.MarkFeedback(time.Unix(2, 0).UTC())

		Convey("It should expose dimensionless floor and peak distances", func() {
			So(feedback.Symbol, ShouldEqual, "SIM/USD")
			So(feedback.Exposed, ShouldBeTrue)
			So(feedback.Mark, ShouldEqual, 100.0)
			So(feedback.FloorDistance, ShouldAlmostEqual, 0.02, 1e-12)
			So(feedback.PeakDrawdown, ShouldAlmostEqual, math.Log(100.0/105.0), 1e-12)
			So(feedback.PnL, ShouldAlmostEqual, 1.25)
			So(feedback.SurgeArmed, ShouldBeTrue)
		})
	})
}

func TestPositionOnExecution(t *testing.T) {
	Convey("Given a position with a strategy stoploss", t, func() {
		store := newPositionStoreFixture(t)
		price, _ := newPriceSurface(t, "SIM/USD")
		price.Update(&kraken.TickerData{
			Symbol: "SIM/USD",
			Bid:    decimal.NewFromFloat64(100),
			Ask:    decimal.NewFromFloat64(100.02),
		})
		position := &Position{
			price: price,
			store: store,
			ui:    newPositionUI(),
			pair: kraken.InstrumentPair{
				Symbol: "SIM/USD",
				Base:   "SIM",
				Quote:  "USD",
			},
			EntryOrder: &spot.AddOrderRequest{ClOrdId: "entry"},
			Holding: &types.Holding{
				Mark:     decimal.NewFromFloat64(100),
				Stoploss: newBrokerStoploss(t),
			},
		}

		Convey("It should save on entry and delete on exit", func() {
			position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
				ClientOrderID: "entry",
				OrderStatus:   "filled",
				Timestamp:     time.Now().UTC(),
				CumQty:        decimal.NewFromFloat64(2),
				CumCost:       decimal.NewFromFloat64(200.04),
				FeeUsdEquiv:   decimal.NewFromFloat64(0.20),
			}}})

			stored, err := store.Load(t.Context(), "SIM/USD")
			So(err, ShouldBeNil)
			So(stored, ShouldNotBeNil)

			position.ExitOrder = &spot.AddOrderRequest{ClOrdId: "exit"}
			exitAt := time.Now().UTC()
			closed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
				ClientOrderID: "exit",
				OrderStatus:   "filled",
				Timestamp:     exitAt,
				CumQty:        decimal.NewFromFloat64(2),
				CumCost:       decimal.NewFromFloat64(220),
				FeeUsdEquiv:   decimal.NewFromFloat64(0.22),
			}}})

			So(closed, ShouldBeTrue)
			So(position.status(), ShouldEqual, types.CLOSED)
			So(position.Holding.Status, ShouldEqual, types.CLOSED)
			So(position.Holding.ExitAt.Equal(exitAt), ShouldBeTrue)
			So(position.Holding.ExitPrice.Cmp(
				decimal.NewFromFloat64(110),
			), ShouldEqual, 0)
			So(position.Holding.PnL.Cmp(
				decimal.NewFromFloat64(19.54),
			), ShouldEqual, 0)
			frame := nextFrame(position.ui)
			So(frame.Type, ShouldEqual, wire.FramePositionsFrame)
			positions := frame.Value.(*wire.PositionsFrameT).Rows
			So(positions, ShouldHaveLength, 1)
			So(positions[0].Status, ShouldEqual, string(types.CLOSED))
			stored, err = store.Load(t.Context(), "SIM/USD")
			So(err, ShouldBeNil)
			So(stored, ShouldBeNil)
		})
	})

	Convey("Given a rejected entry with no fill economics", t, func() {
		position := &Position{
			ui:         newPositionUI(),
			EntryOrder: &spot.AddOrderRequest{ClOrdId: "entry"},
			Holding:    &types.Holding{},
		}
		position.setStatus(types.PENDING)

		removed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
			ClientOrderID: "entry",
			OrderStatus:   "rejected",
		}}})

		Convey("It should publish the terminal state and release the desk slot", func() {
			So(removed, ShouldBeTrue)
			So(position.status(), ShouldEqual, types.REJECTED)
			So(position.Holding.Status, ShouldEqual, types.REJECTED)
		})
	})

	Convey("Given a zero-fill canceled exit", t, func() {
		stoploss := newBrokerStoploss(t)
		stoploss.Update(stoploss.Floor.Sub(decimal.NewFromFloat64(0.01)))
		position := &Position{
			ui:        newPositionUI(),
			ExitOrder: &spot.AddOrderRequest{ClOrdId: "exit"},
			Holding: &types.Holding{
				Qty:      decimal.NewFromInt64(1),
				Stoploss: stoploss,
			},
		}
		position.setStatus(types.PENDING)

		removed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
			ClientOrderID: "exit",
			OrderStatus:   "canceled",
			CumQty:        decimal.NewFromInt64(0),
		}}})

		Convey("It should preserve inventory and reopen the submission boundary", func() {
			So(removed, ShouldBeFalse)
			So(position.ExitOrder, ShouldBeNil)
			So(position.status(), ShouldEqual, types.OPEN)
			So(position.Holding.Status, ShouldEqual, types.OPEN)
			So(position.Holding.Stoploss.Status, ShouldEqual, types.TRIGGERED)
		})
	})

	Convey("Given a terminal exit with partial filled quantity", t, func() {
		position := &Position{
			ui:        newPositionUI(),
			ExitOrder: &spot.AddOrderRequest{ClOrdId: "exit"},
			Holding: &types.Holding{
				Qty: decimal.NewFromInt64(1),
			},
		}
		position.setStatus(types.PENDING)

		removed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
			ClientOrderID: "exit",
			OrderStatus:   "expired",
			CumQty:        decimal.NewFromFloat64(0.25),
		}}})

		Convey("It should expose the unresolved inventory instead of overselling", func() {
			So(removed, ShouldBeFalse)
			So(position.ExitOrder, ShouldNotBeNil)
			So(position.status(), ShouldEqual, types.ERROR)
			So(position.Holding.Status, ShouldEqual, types.ERROR)
		})
	})

	Convey("Given a closed position receiving subsequent duplicate exit executions", t, func() {
		position := &Position{
			ui:        newPositionUI(),
			ExitOrder: &spot.AddOrderRequest{ClOrdId: "exit"},
			Holding: &types.Holding{
				Qty:         decimal.NewFromInt64(1),
				SellableQty: decimal.NewFromInt64(0),
				Status:      types.CLOSED,
			},
		}
		position.setStatus(types.CLOSED)

		removed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
			ClientOrderID: "exit",
			OrderStatus:   "filled",
			Timestamp:     time.Now().UTC(),
			CumQty:        decimal.NewFromInt64(1),
			CumCost:       decimal.NewFromFloat64(100),
			FeeUsdEquiv:   decimal.NewFromFloat64(0.25),
		}}})

		Convey("It should return true idempotently without erroring", func() {
			So(removed, ShouldBeTrue)
			So(position.status(), ShouldEqual, types.CLOSED)
			So(position.Holding.Status, ShouldEqual, types.CLOSED)
		})
	})

	Convey("Given duplicate execution identities for the same order", t, func() {
		position := &Position{
			ui:             newPositionUI(),
			ExitOrder:      &spot.AddOrderRequest{ClOrdId: "exit"},
			seenExecutions: map[string]struct{}{"EXEC-1": {}},
			Holding: &types.Holding{
				Qty:         decimal.NewFromInt64(1),
				SellableQty: decimal.NewFromInt64(1),
				Status:      types.PENDING,
			},
		}
		position.setStatus(types.PENDING)

		removed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
			ExecID:        "EXEC-1",
			ClientOrderID: "exit",
			OrderStatus:   "filled",
			Timestamp:     time.Now().UTC(),
			CumQty:        decimal.NewFromInt64(1),
			CumCost:       decimal.NewFromFloat64(100),
			FeeUsdEquiv:   decimal.NewFromFloat64(0.25),
		}}})

		Convey("It should skip the duplicate execution identity", func() {
			So(removed, ShouldBeFalse)
			So(position.status(), ShouldEqual, types.PENDING)
		})
	})
}

func TestPositionExit(t *testing.T) {
	Convey("Given an open lot whose regulator remains armed", t, func() {
		position := &Position{
			Holding: &types.Holding{
				Qty:      decimal.NewFromInt64(1),
				Stoploss: newBrokerStoploss(t),
			},
		}
		position.setStatus(types.OPEN)

		Convey("It should reject liquidation at the order boundary", func() {
			returned, err := position.Exit()

			So(returned, ShouldEqual, position)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "triggered stoploss required")
			So(position.ExitOrder, ShouldBeNil)
		})
	})

	Convey("Given a triggered lot whose first sell submission fails", t, func() {
		private := &retryExitConn{Conn: mock.NewConn(), failures: 1}
		api := websocket.NewAPI(t.Context(), mock.NewConn(), private)
		stoploss := newBrokerStoploss(t)
		stoploss.Update(stoploss.Floor.Sub(decimal.NewFromFloat64(0.01)))
		position := &Position{
			api:        api,
			EntryOrder: &spot.AddOrderRequest{ClOrdId: "entry"},
			pair:       kraken.InstrumentPair{Symbol: "SIM/USD"},
			Holding: &types.Holding{
				Qty:      decimal.NewFromInt64(1),
				Stoploss: stoploss,
			},
		}
		position.setStatus(types.OPEN)

		_, firstErr := position.Exit()
		_, secondErr := position.Exit()

		Convey("It should preserve the trigger and retry the same exit on the next opportunity", func() {
			So(firstErr, ShouldNotBeNil)
			So(secondErr, ShouldBeNil)
			So(stoploss.Status, ShouldEqual, types.TRIGGERED)
			So(position.ExitOrder, ShouldNotBeNil)
			So(position.status(), ShouldEqual, types.PENDING)
			So(private.attempts, ShouldEqual, 2)
		})
	})
}

type retryExitConn struct {
	*mock.Conn
	failures int
	attempts int
}

func (conn *retryExitConn) AddOrder(
	_ *spot.AddOrderRequest,
) (spot.AddOrderResult, error) {
	conn.attempts++

	if conn.attempts <= conn.failures {
		return spot.AddOrderResult{}, errors.New("temporary exit rejection")
	}

	return spot.AddOrderResult{}, nil
}

func TestPositionCloseFill(t *testing.T) {
	Convey("Given an exact filled exit for a tiny high-priced lot", t, func() {
		quantity := decimal.NewFromFloat64(0.00051057)
		entryPrice := decimal.NewFromFloat64(64951.1)
		entryGross := decimal.NewFromInt64(0).Add(entryPrice).Mul(quantity)
		entryFee := entryGross.Mul(
			decimal.NewFromFloat64(0.0025),
		)
		exitPrice := decimal.NewFromFloat64(66000)
		exitGross := decimal.NewFromInt64(0).Add(exitPrice).Mul(quantity)
		exitFee := exitGross.Mul(
			decimal.NewFromFloat64(0.0025),
		)
		position := &Position{
			ui: newPositionUI(),
			Holding: &types.Holding{
				Status:      types.OPEN,
				Qty:         quantity,
				SellableQty: quantity,
				EntryPrice:  entryPrice,
				EntryFee:    entryFee,
			},
		}
		position.setStatus(types.OPEN)
		execution := kraken.ExecutionData{
			Timestamp:   time.Now().UTC(),
			CumQty:      quantity,
			CumCost:     exitGross,
			FeeUsdEquiv: exitFee,
		}

		Convey("It should retain realized proceeds and fees before publishing closed", func() {
			err := position.closeFill(execution)
			expectedPnL := 66000*quantity.Float64()*(1-0.0025) -
				64951.1*quantity.Float64()*(1+0.0025)

			So(err, ShouldBeNil)
			So(position.Holding.ExitPrice.Cmp(exitPrice), ShouldEqual, 0)
			So(position.Holding.PnL.Float64(), ShouldAlmostEqual, expectedPnL, 1e-10)
			So(position.Holding.ReturnPct, ShouldBeGreaterThan, 0)
			So(position.Holding.ReturnPct, ShouldBeLessThan, 2)
			So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
			So(position.status(), ShouldEqual, types.CLOSED)
		})

		Convey("It should fall back to Qty when SellableQty is zero or nil", func() {
			position.Holding.SellableQty = decimal.NewFromInt64(0)

			err := position.closeFill(execution)

			So(err, ShouldBeNil)
			So(position.status(), ShouldEqual, types.CLOSED)
		})

		Convey("It should refuse a filled quantity that does not match inventory", func() {
			execution.CumQty = quantity.Add(quantity.GetSmallestIncrement())

			err := position.closeFill(execution)

			So(err, ShouldNotBeNil)
			So(position.status(), ShouldEqual, types.OPEN)
			So(position.Holding.ExitPrice, ShouldBeNil)
		})

		Convey("It should refuse a filled event without realized cost", func() {
			execution.CumCost = nil

			err := position.closeFill(execution)

			So(err, ShouldNotBeNil)
			So(position.status(), ShouldEqual, types.OPEN)
			So(position.Holding.ExitPrice, ShouldBeNil)
		})
	})
}

/*
nextFrame drains one wire frame from a test ui transport, registering a local
consumer cursor so publications exist before the behavior under test runs.
*/
var positionTestConsumer = transport.NewConsumer[*types.UIFrame](
	"position-test", func() {},
)

func newPositionUI() *transport.MapReduce[*types.UIFrame] {
	return transport.NewMapReduce[*types.UIFrame](
		[]*transport.Consumer[*types.UIFrame]{positionTestConsumer},
		nil,
		nil,
	)
}

func nextFrame(ui *transport.MapReduce[*types.UIFrame]) *types.UIFrame {
	var frame *types.UIFrame

	for candidate := range ui.Drain(positionTestConsumer, nil) {
		frame = candidate
	}

	return frame
}

func BenchmarkPositionOnExecution(b *testing.B) {
	price, _ := newPriceSurface(b, "SIM/USD")
	price.Update(&kraken.TickerData{
		Symbol: "SIM/USD",
		Bid:    decimal.NewFromFloat64(100),
		Ask:    decimal.NewFromFloat64(100.02),
	})
	execution := kraken.Execution{Data: []kraken.ExecutionData{{
		ExecID:        "BENCH-1",
		ClientOrderID: "entry",
		OrderStatus:   "filled",
		Timestamp:     time.Now().UTC(),
		CumQty:        decimal.NewFromFloat64(2),
		CumCost:       decimal.NewFromFloat64(200.04),
		FeeUsdEquiv:   decimal.NewFromFloat64(0.20),
	}}}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		position := &Position{
			price:          price,
			ui:             newPositionUI(),
			EntryOrder:     &spot.AddOrderRequest{ClOrdId: "entry"},
			seenExecutions: map[string]struct{}{},
			Holding: &types.Holding{
				Mark:     decimal.NewFromFloat64(100),
				Stoploss: newBrokerStoploss(b),
			},
		}
		position.onExecution(execution)
	}
}

func BenchmarkPositionCloseFill(b *testing.B) {
	quantity := decimal.NewFromFloat64(1)
	entryPrice := decimal.NewFromFloat64(100)
	entryFee := decimal.NewFromFloat64(0.25)
	execution := kraken.ExecutionData{
		Timestamp:   time.Now().UTC(),
		CumQty:      quantity,
		CumCost:     decimal.NewFromFloat64(110),
		FeeUsdEquiv: decimal.NewFromFloat64(0.25),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		position := &Position{
			ui: newPositionUI(),
			Holding: &types.Holding{
				Status:      types.OPEN,
				Qty:         quantity,
				SellableQty: quantity,
				EntryPrice:  entryPrice,
				EntryFee:    entryFee,
			},
		}
		position.setStatus(types.OPEN)
		_ = position.closeFill(execution)
	}
}
