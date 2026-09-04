package broker

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
mustDecimal parses an exact decimal string and panics on failure. Test fixtures
use it so float64 representation error never leaks into accounting assertions.
*/
func mustDecimal(value string) *decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		panic(err)
	}

	return parsed
}

/*
closeFillPosition builds a minimal open lot whose realized exit economics can be
asserted without a live exchange, desk, or database. sellable is the complete
filled inventory; entryPrice and entryFee are the authoritative entry basis.
*/
func closeFillPosition(sellable string, entryPrice string, entryFee string) *Position {
	quantity, _ := decimal.NewFromString(sellable)
	price, _ := decimal.NewFromString(entryPrice)
	fee, _ := decimal.NewFromString(entryFee)

	position := &Position{
		pair: kraken.InstrumentPair{Symbol: "SHAPE/USD"},
		Decision: types.Decision{
			ID:     "shape-decision",
			Symbol: "SHAPE/USD",
		},
		Holding: &types.Holding{
			Symbol:      "SHAPE/USD",
			Qty:         quantity,
			SellableQty: quantity,
			EntryPrice:  price,
			EntryFee:    fee,
		},
	}

	position.setStatus(types.OPEN)

	return position
}

func TestPositionWire(t *testing.T) {
	Convey("Given an open position retaining its entry decision", t, func() {
		position := closeFillPosition("100000", "0.000490", "0.04")
		position.Decision.Action = types.ActionEnter
		position.Decision.Reason = "forecast clears fee-inclusive break-even"
		position.Decision.Confidence = 0.73
		position.decisionWire = types.DecisionWire(&position.Decision)

		Convey("the wire snapshot includes that exact frozen decision", func() {
			encoded := position.Wire()

			So(encoded.Decision, ShouldNotBeNil)
			So(encoded.Decision.Id, ShouldEqual, "shape-decision")
			So(encoded.Decision.Action, ShouldEqual, "enter")
			So(encoded.Decision.Reason, ShouldEqual, "forecast clears fee-inclusive break-even")
			So(encoded.Decision.Confidence, ShouldEqual, 0.73)
		})
	})
}

/*
executionFixture builds a Kraken ExecutionData with a last individual fill whose
LastPrice is materially different from the whole-order AvgPrice, so a
close-fill path that marks the exit by the last fill would diverge from the
authoritative whole-order realized VWAP. All figures are exact decimal strings
so float64 representation error never leaks into the assertion.
*/
func executionFixture(
	lastPrice string,
	avgPrice string,
	cumQty string,
	cumCost string,
	feeUsd string,
) kraken.ExecutionData {
	return kraken.ExecutionData{
		ExecID:      "exit-1",
		LastQty:     mustDecimal(cumQty),
		LastPrice:   mustDecimal(lastPrice),
		CumQty:      mustDecimal(cumQty),
		CumCost:     mustDecimal(cumCost),
		AvgPrice:    mustDecimal(avgPrice),
		FeeUsdEquiv: mustDecimal(feeUsd),
		Timestamp:   time.Now(),
		OrderStatus: "filled",
	}
}

func TestCloseFillWholeOrderVWAP(t *testing.T) {
	Convey("Given a multi-fill exit where the last fill differs from the whole-order average", t, func() {
		// Whole-order realized VWAP 0.000506, final individual fill 0.000520:
		// marking the exit by LastPrice would overstate proceeds and fake a
		// profit the realized economics do not support.
		position := closeFillPosition("100000", "0.000490", "0.04")
		execution := executionFixture("0.000520", "0.000506", "100000", "50.6", "0.40")
		sellable := mustDecimal("100000")
		avgPrice := mustDecimal("0.000506")

		Convey("the exit price is the whole-order realized VWAP, not the last fill", func() {
			err := position.closeFill(execution)

			So(err, ShouldBeNil)
			So(position.Holding.ExitVWAP.Cmp(avgPrice), ShouldEqual, 0)
			So(position.Holding.ExitPrice.Cmp(avgPrice), ShouldEqual, 0)
			So(position.Holding.ExitQty.Cmp(sellable), ShouldEqual, 0)
		})

		Convey("PnL and ReturnPct reconcile from the same realized economics", func() {
			err := position.closeFill(execution)

			So(err, ShouldBeNil)

			// entry basis 0.000490 × 100000 = 49, + entry fee 0.04 = 49.04
			// exit proceeds 50.6 − exit fee 0.40 = 50.2
			// realized PnL = 50.2 − 49.04 = 1.16
			So(position.Holding.PnL.Float64(), ShouldAlmostEqual, 1.16, 1e-12)
			So(position.Holding.RealizedPnL.Float64(), ShouldAlmostEqual, 1.16, 1e-12)

			expectedReturn := 1.16 / 49.04 * 100
			So(position.Holding.ReturnPct, ShouldAlmostEqual, expectedReturn, 1e-9)

			// RealizedReturn is the fee-inclusive fraction derived from the same
			// realized economics; it reconciles with ReturnPct to within the
			// decimal library's scale.
			realizedPct := position.Holding.RealizedReturn.Float64() * 100
			So(realizedPct-expectedReturn < 1e-8 && expectedReturn-realizedPct < 1e-8, ShouldBeTrue)
		})

		Convey("exit fees are the exchange's authoritative total", func() {
			err := position.closeFill(execution)

			So(err, ShouldBeNil)
			So(position.Holding.ExitFees.Cmp(mustDecimal("0.40")), ShouldEqual, 0)
			So(position.Holding.ExitFee.Cmp(mustDecimal("0.40")), ShouldEqual, 0)
		})
	})
}

func TestCloseFillAvgPriceFallback(t *testing.T) {
	Convey("Given an exit execution without an explicit AvgPrice", t, func() {
		position := closeFillPosition("100000", "0.000490", "0.04")
		execution := executionFixture("0.000520", "0", "100000", "50.6", "0.40")
		execution.AvgPrice = nil

		Convey("the exit VWAP falls back to the cumulative CumCost/CumQty equivalent", func() {
			err := position.closeFill(execution)

			So(err, ShouldBeNil)
			So(position.Holding.ExitVWAP.Cmp(mustDecimal("0.000506")), ShouldEqual, 0)
		})
	})
}

/*
partialFillPosition builds a lot mid-exit: the ExitOrder is already submitted and
the sellable inventory is complete, so onExecution routes exit executions to the
close path.
*/
func partialFillPosition(sellable string, entryPrice string, entryFee string) *Position {
	position := closeFillPosition(sellable, entryPrice, entryFee)
	position.ExitOrder = &spot.AddOrderRequest{
		ClOrdId: "shape-entry-exit",
		Type:    "sell",
		Volume:  sellable,
		Pair:    "SHAPE/USD",
	}

	return position
}

func TestPartialFillExitAccumulatesWholeOrder(t *testing.T) {
	Convey("Given a multi-fill exit with a duplicate terminal fill", t, func() {
		position := partialFillPosition("100000", "0.000490", "0.04")

		partialOne := executionFixture("0.000510", "0.000510", "40000", "20.4", "0.10")
		partialOne.ExecID = "exit-partial-1"
		partialOne.OrderStatus = "partially_filled"
		partialOne.ClientOrderID = "shape-entry-exit"

		partialTwo := executionFixture("0.000505", "0.000505", "60000", "30.3", "0.15")
		partialTwo.ExecID = "exit-partial-2"
		partialTwo.OrderStatus = "partially_filled"
		partialTwo.ClientOrderID = "shape-entry-exit"

		terminal := executionFixture("0.000502", "0.000504", "100000", "50.4", "0.25")
		terminal.ExecID = "exit-terminal"
		terminal.ClientOrderID = "shape-entry-exit"
		terminal.OrderStatus = "filled"

		dupe := terminal
		dupe.ExecID = "exit-terminal"

		Convey("the terminal fill's cumulative whole-order VWAP is the exit price, and duplicates do not double-count", func() {
			finished := position.onExecution(kraken.Execution{
				Channel: "executions",
				Type:    "update",
				Data: []kraken.ExecutionData{
					partialOne, partialTwo, terminal, dupe,
				},
			})

			So(finished, ShouldBeTrue)
			So(position.Holding.ExitVWAP.Cmp(mustDecimal("0.000504")), ShouldEqual, 0)
			So(position.Holding.ExitQty.Cmp(mustDecimal("100000")), ShouldEqual, 0)
			So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)

			// 0.000490 × 100000 = 49, + 0.04 fee = 49.04 basis.
			// 50.4 − 0.25 = 50.15 proceeds. PnL = 1.11.
			So(position.Holding.PnL.Float64(), ShouldAlmostEqual, 1.11, 1e-9)
		})
	})
}

/*
triggeredExitPosition builds an open lot whose stoploss has already latched
TRIGGERED, mirroring the state Exit() requires before it will submit anything.
*/
func triggeredExitPosition(t *testing.T, private *mockConn) *Position {
	api := websocket.NewAPI(t.Context(), newMockConn(), private)

	position := &Position{
		pair: kraken.InstrumentPair{Symbol: "SHAPE/USD"},
		api:  api,
		Decision: types.Decision{
			ID:     "shape-decision",
			Symbol: "SHAPE/USD",
		},
		EntryOrder: &spot.AddOrderRequest{
			ClOrdId: "shape-entry",
			Type:    "buy",
			Volume:  "100000",
			Pair:    "SHAPE/USD",
		},
		Holding: &types.Holding{
			Symbol:      "SHAPE/USD",
			Qty:         mustDecimal("100000"),
			SellableQty: mustDecimal("100000"),
			EntryPrice:  mustDecimal("0.000490"),
			EntryFee:    mustDecimal("0.04"),
			Stoploss: &types.Stoploss{
				Status: types.TRIGGERED,
				Symbol: "SHAPE/USD",
				Locked: true,
			},
		},
	}

	position.setStatus(types.OPEN)

	return position
}

/*
A triggered stop is only meaningful if Exit() gets another chance after a
failed submission — otherwise a single transient AddOrder error (a dropped
connection, an exchange-side rejection) permanently strands the position: the
stop stays latched TRIGGERED forever with no order ever in flight, while price
keeps moving underneath it. exitClaim used to latch true on the first attempt
and never release on failure, so every subsequent tick's Exit() call would
silently no-op instead of retrying.
*/
func TestExitRetriesAfterFailedSubmission(t *testing.T) {
	Convey("Given a triggered stop whose first exit submission fails", t, func() {
		private := newMockConn()
		private.AddOrderErr = errors.New("exchange rejected order")
		position := triggeredExitPosition(t, private)

		_, err := position.Exit()
		So(err, ShouldNotBeNil)
		So(position.ExitOrder, ShouldBeNil)

		Convey("a later tick can retry and submit the exit successfully", func() {
			private.AddOrderErr = nil

			_, err := position.Exit()

			So(err, ShouldBeNil)
			So(position.ExitOrder, ShouldNotBeNil)
			So(position.ExitOrder.ClOrdId, ShouldNotEqual, position.EntryOrder.ClOrdId)
			So(position.ExitOrder.ClOrdId, ShouldEqual, uuid.NewSHA1(
				uuid.NameSpaceOID,
				[]byte("symm:exit:"+position.EntryOrder.ClOrdId),
			).String())
			_, uuidErr := uuid.Parse(position.ExitOrder.ClOrdId)
			So(uuidErr, ShouldBeNil)
			So(position.Holding.Status, ShouldEqual, types.PENDING)
		})
	})
}

func TestPositionOnExecutionTerminalPartialEntry(t *testing.T) {
	Convey("Given a live entry that partially fills before Kraken cancels its remainder", t, func() {
		conn := newExecutingConn(nil)
		desk, workload := newDeliveryDesk(t, conn)
		conn.workload = workload
		desk.price.fees.Store("TEST/USD", kraken.TradeVolumeFee{Fee: mustDecimal("0.25")})
		decision := types.Decision{
			ID:               uuid.NewString(),
			Action:           types.ActionEnter,
			Symbol:           "TEST/USD",
			At:               time.Now(),
			ProposedQuantity: mustDecimal("100"),
			ProposedNotional: mustDecimal("200.00"),
			ForecastHorizon:  1,
			Mark:             mustDecimal("2.00"),
		}
		stoploss, err := types.NewStoplossWithPlan(
			t.Context(),
			"TEST/USD",
			mustDecimal("2.00"),
			mustDecimal("2.00"),
			&learning.RLSOutput{Ready: true},
			0,
			mustDecimal("0.01"),
			mustDecimal("0.008"),
			mustDecimal("0.008"),
			nil,
			time.Now(),
		)
		So(err, ShouldBeNil)
		decision.Stoploss = stoploss
		position := NewPosition(
			t.Context(), desk.api, desk.instrument, desk.price, desk.balance,
			desk.PositionStore,
			kraken.InstrumentPair{
				Symbol:   "TEST/USD",
				Base:     "TEST",
				Quote:    "USD",
				TickSize: *mustDecimal("0.01"),
			},
			decision,
		)
		position.setStatus(types.PENDING)
		execution := kraken.ExecutionData{
			OrderID:       "venue-entry",
			ClientOrderID: decision.ID,
			ExecID:        "entry-terminal-partial",
			ExecType:      "canceled",
			Symbol:        "TEST/USD",
			Side:          "buy",
			LastQty:       mustDecimal("40"),
			LastPrice:     mustDecimal("2.00"),
			CumQty:        mustDecimal("40"),
			CumCost:       mustDecimal("80.00"),
			AvgPrice:      mustDecimal("2.00"),
			FeeUsdEquiv:   mustDecimal("0.20"),
			Timestamp:     time.Now(),
			OrderStatus:   "canceled",
		}

		Convey("the filled inventory remains open and protected", func() {
			finished := position.onExecution(kraken.Execution{
				Channel: "executions",
				Type:    "update",
				Data:    []kraken.ExecutionData{execution},
			})

			So(finished, ShouldBeFalse)
			So(position.status(), ShouldEqual, types.OPEN)
			So(position.Holding.Status, ShouldEqual, types.OPEN)
			So(position.Holding.Qty.Cmp(mustDecimal("40")), ShouldEqual, 0)
			So(position.Holding.SellableQty.Cmp(mustDecimal("40")), ShouldEqual, 0)
			So(position.Holding.EntryPrice.Cmp(mustDecimal("2.00")), ShouldEqual, 0)
			So(position.Holding.EntryFee.Cmp(mustDecimal("0.20")), ShouldEqual, 0)
			So(position.Holding.Stoploss.Status, ShouldEqual, types.ARMED)
		})

		Convey("a later terminal status without trade fields retains the prior partial fill", func() {
			execution.OrderStatus = "partially_filled"
			execution.ExecID = "entry-partial"
			So(position.onExecution(kraken.Execution{
				Channel: "executions",
				Type:    "update",
				Data:    []kraken.ExecutionData{execution},
			}), ShouldBeFalse)

			terminal := kraken.ExecutionData{
				OrderID:       execution.OrderID,
				ClientOrderID: decision.ID,
				ExecID:        "entry-canceled",
				ExecType:      "canceled",
				Symbol:        "TEST/USD",
				Side:          "buy",
				Timestamp:     time.Now(),
				OrderStatus:   "canceled",
			}

			So(position.onExecution(kraken.Execution{
				Channel: "executions",
				Type:    "update",
				Data:    []kraken.ExecutionData{terminal},
			}), ShouldBeFalse)
			So(position.status(), ShouldEqual, types.OPEN)
			So(position.Holding.Status, ShouldEqual, types.OPEN)
			So(position.Holding.Qty.Cmp(mustDecimal("40")), ShouldEqual, 0)
			So(position.Holding.SellableQty.Cmp(mustDecimal("40")), ShouldEqual, 0)
			So(position.Holding.Stoploss.Status, ShouldEqual, types.ARMED)
		})
	})
}

/*
The operator's manual EXIT button dispatches through the guardian ring
(ManualExit -> publishGuardian -> handleGuardian -> executeManualExit), and
handleGuardian used to discard executeManualExit's error entirely — a click
that failed for any reason (a stop not in an overridable state, a rejected
order, an already-claimed exit) looked identical to a click that worked: no
log, no error, the button just stuck on "EXITING" forever. This asserts the
failure is no longer silent — executeManualExit itself returns a real error
that a caller (or handleGuardian's log) can observe.
*/
func TestExecuteManualExitReportsFailure(t *testing.T) {
	Convey("Given a lot whose stoploss cannot be manually overridden", t, func() {
		private := newMockConn()
		position := triggeredExitPosition(t, private)
		// PENDING is neither ARMED (overridable) nor TRIGGERED (already an
		// exit path) — TriggerManualOverride must refuse it.
		position.Holding.Stoploss.Status = types.PENDING

		Convey("executeManualExit surfaces the rejection instead of pretending success", func() {
			err := position.executeManualExit()

			So(err, ShouldNotBeNil)
			So(position.ExitOrder, ShouldBeNil)
		})
	})
}

func BenchmarkPositionWire(b *testing.B) {
	position := closeFillPosition("100000", "0.000490", "0.04")
	position.Decision.Action = types.ActionEnter
	position.Decision.Reason = "forecast clears fee-inclusive break-even"
	position.Decision.Confidence = 0.73
	position.Decision.Alternatives = map[string]float64{
		"probability:profitable": 0.73,
		"probability:up":         0.81,
		"return:break_even_log":  0.005,
		"return:expected_log":    0.018,
	}
	position.decisionWire = types.DecisionWire(&position.Decision)

	b.ResetTimer()

	for range b.N {
		_ = position.Wire()
	}
}
