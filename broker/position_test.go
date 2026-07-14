package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

func TestPositionOrderAck(t *testing.T) {
	Convey("Given a position waiting for one request identity", t, func() {
		ui := make(chan []byte, 8)
		position := &Position{
			reqID: 7,
			ui:    ui,
			Data:  &PositionData{Symbol: "BTC/USD"},
		}

		position.OrderAck([]byte(`{
			"method":"add_order",
			"result":{"order_id":"wrong"},
			"success":true,
			"req_id":8
		}`))

		Convey("It should ignore another position's acknowledgement", func() {
			So(position.orderID, ShouldBeEmpty)
			So(len(ui), ShouldEqual, 0)
		})

		position.OrderAck([]byte(`{
			"method":"add_order",
			"result":{"order_id":"right"},
			"success":true,
			"req_id":7
		}`))

		Convey("It should accept and publish its own acknowledgement", func() {
			So(position.orderID, ShouldEqual, "right")
			So(position.Status(), ShouldEqual, types.OPEN)
			So(len(ui), ShouldEqual, 1)
		})
	})
}

/*
TestPositionTickerAck verifies that ticker delivery cannot dereference an
uninitialized SDK decimal while a restored position is still reconciling.
*/
func TestPositionTickerAck(t *testing.T) {
	frame := []byte(`{
		"channel":"ticker",
		"type":"update",
		"data":[{"symbol":"NPC/USD","last":"110"}]
	}`)

	Convey("Given a restored position without a reconciled entry price", t, func() {
		position := &Position{
			status: types.OPEN,
			Data: &PositionData{
				Symbol: "NPC/USD",
				Qty:    *decimal.NewFromInt64(1),
			},
		}

		position.TickerAck(frame)

		Convey("Then validation rejects it without publishing or panicking", func() {
			So(position.Status(), ShouldEqual, types.ERROR)
			So(position.Data.Mark.Rat().Sign(), ShouldEqual, 0)
		})
	})

	Convey("Given a reconciled position with a fee schedule", t, func() {
		price := &Price{status: types.READY, fees: &sync.Map{}}
		price.fees.Store("NPC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		ui := make(chan []byte, 1)
		position := &Position{
			status: types.OPEN,
			price:  price,
			ui:     ui,
			Data: &PositionData{
				Symbol:     "NPC/USD",
				Qty:        *decimal.NewFromInt64(1),
				EntryPrice: *decimal.NewFromInt64(100),
			},
		}

		position.TickerAck(frame)

		Convey("Then the ticker updates and publishes the current valuation", func() {
			So(position.Data.Mark.Float64(), ShouldEqual, 110.0)
			So(position.Data.PnL.Float64(), ShouldAlmostEqual, 9.454)
			So(len(ui), ShouldEqual, 1)
		})
	})
}

func TestPositionExecutionAck(t *testing.T) {
	Convey("Given a position receiving cumulative buy fills", t, func() {
		ui := make(chan []byte, 8)
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.status = types.READY
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.26"})
		thesis := types.NewThesis(nil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleShaped, time.Unix(1, 0),
		), ShouldBeNil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleEntrySelected, time.Unix(2, 0),
		), ShouldBeNil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleEntrySubmitted, time.Unix(3, 0),
		), ShouldBeNil)
		position := &Position{
			orderID: "order-1",
			ui:      ui,
			price:   price,
			thesis:  thesis,
			Data: &PositionData{
				Symbol: "BTC/USD",
			},
		}

		position.ExecutionAck([]byte(`{
			"channel":"executions",
			"type":"update",
			"data":[{
				"order_id":"order-1",
				"exec_id":"fill-2",
				"exec_type":"trade",
				"symbol":"BTC/USD",
				"side":"buy",
				"last_qty":1,
				"last_price":"110",
				"cost":"110",
				"order_status":"filled",
				"cum_qty":2,
				"cum_cost":"210",
				"avg_price":"105"
			}]
		}`))

		Convey("It should use exchange cumulative quantity and average price", func() {
			So(position.Data.Qty.Float64(), ShouldEqual, 2.0)
			So(position.Data.EntryPrice.Float64(), ShouldEqual, 105.0)
			So(position.Data.Mark.Float64(), ShouldEqual, 110.0)
			So(position.Status(), ShouldEqual, types.FILLED)
			So(position.Executions(), ShouldHaveLength, 1)
			So(thesis.TradeJournal, ShouldHaveLength, 5)
			So(thesis.TradeJournal[4].Kind, ShouldEqual, "execution")
			So(thesis.TradeJournal[4].ExecutionID, ShouldEqual, "fill-2")
			So(thesis.TradeJournal[4].Quantity, ShouldEqual, "1")
			So(thesis.TradeJournal[4].Price, ShouldEqual, "110")
			So(len(ui), ShouldEqual, 1)
		})
	})
}

func TestPositionExecutionAckClosesWithoutDiscardingLifecycle(t *testing.T) {
	Convey("Given the final sell fill for a Thesis-backed position", t, func() {
		thesis := types.NewThesis(nil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleManaging, time.Unix(1, 0),
		), ShouldBeNil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleExitSelected, time.Unix(2, 0),
		), ShouldBeNil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleExitSubmitted, time.Unix(3, 0),
		), ShouldBeNil)
		position := &Position{
			orderID:       "exit-1",
			requestedQty:  *decimal.NewFromInt64(1),
			priorQty:      *decimal.NewFromInt64(1),
			currentAction: "exit",
			ui:            make(chan []byte, 1),
			price:         &Price{},
			thesis:        thesis,
			executions: []*kraken.Execution{{Data: []kraken.ExecutionData{{
				ExecID: "entry-fill", ExecType: "trade", Symbol: "BTC/USD", Side: "buy",
				LastQty: 1, LastPrice: *decimal.NewFromInt64(100),
				Cost: *decimal.NewFromInt64(100), FeeUsdEquiv: *decimal.NewFromInt64(1),
			}}}},
			Data: &PositionData{
				Symbol:     "BTC/USD",
				Qty:        *decimal.NewFromInt64(1),
				EntryPrice: *decimal.NewFromInt64(100),
			},
		}

		position.ExecutionAck([]byte(`{
			"channel":"executions",
			"type":"update",
			"data":[{
				"order_id":"exit-1",
				"exec_id":"exit-fill",
				"exec_type":"trade",
				"symbol":"BTC/USD",
				"side":"sell",
				"last_qty":1,
				"last_price":"110",
				"cost":"110",
				"order_status":"filled",
				"cum_qty":1,
				"cum_cost":"110",
				"avg_price":"110",
				"fee_usd_equiv":"1",
				"timestamp":"2026-07-14T10:00:00Z"
			}]
		}`))

		Convey("Then the runtime position closes while its Thesis retains the fill", func() {
			So(position.Status(), ShouldEqual, types.CLOSED)
			So(position.Data.Qty.Sign(), ShouldEqual, 0)
			So(position.Thesis(), ShouldEqual, thesis)
			So(position.Data.PnL.Float64(), ShouldEqual, 8.0)
			So(position.Data.ReturnPct, ShouldAlmostEqual, 7.9207920792, 0.0000001)
			So(thesis.TradeJournal, ShouldHaveLength, 7)
			So(thesis.TradeJournal[4].ExecutionID, ShouldEqual, "exit-fill")
			So(thesis.TradeJournal[5].Kind, ShouldEqual, "final_outcome")
			So(thesis.TradeJournal[5].PnL, ShouldEqual, "8.000000000000")
			So(thesis.TradeJournal[6].Kind, ShouldEqual, "position_snapshot")
			So(thesis.TradeJournal[6].Status, ShouldEqual, "closed")
		})
	})
}

func TestPositionExecutionAckReduces(t *testing.T) {
	Convey("Given a filled reduction smaller than the open position", t, func() {
		thesis := types.NewThesis(nil)
		So(thesis.Transition(
			"BTC/USD", types.LifecycleManaging, time.Unix(1, 0),
		), ShouldBeNil)
		position := &Position{
			orderID:       "reduce-1",
			requestedQty:  *decimal.NewFromInt64(1),
			priorQty:      *decimal.NewFromInt64(2),
			currentAction: "reduce",
			ui:            make(chan []byte, 1),
			thesis:        thesis,
			Data: &PositionData{
				Symbol: "BTC/USD", Qty: *decimal.NewFromInt64(2),
				EntryPrice: *decimal.NewFromInt64(0),
			},
		}

		position.ExecutionAck([]byte(`{
			"channel":"executions",
			"type":"update",
			"data":[{
				"order_id":"reduce-1",
				"exec_id":"reduce-fill",
				"exec_type":"trade",
				"symbol":"BTC/USD",
				"side":"sell",
				"last_qty":1,
				"last_price":"110",
				"cost":"110",
				"order_status":"filled",
				"cum_qty":1,
				"cum_cost":"110",
				"avg_price":"110",
				"fee_usd_equiv":"1",
				"timestamp":"2026-07-14T10:00:00Z"
			}]
		}`))

		Convey("Then only the requested quantity is removed and management continues", func() {
			So(position.Data.Qty.Float64(), ShouldEqual, 1.0)
			So(position.Status(), ShouldEqual, types.OPEN)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleManaging)
			So(thesis.TradeJournal, ShouldHaveLength, 3)
			So(thesis.TradeJournal[2].Kind, ShouldEqual, "position_snapshot")
			So(thesis.TradeJournal[2].Action, ShouldEqual, "reduce")
		})
	})
}

func TestPositionHydrate(t *testing.T) {
	Convey("Given a wallet holding several assets and trade history for two of them", t, func() {
		ui := make(chan []byte, 8)
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.status = types.READY
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		price.tickers.Store("BTC/USD", &kraken.TickerData{
			Symbol: "BTC/USD",
			Last:   decimal.NewFromFloat64(63039.400),
		})

		balance := &Balance{
			quote: "USD",
			model: &kraken.Balance{
				Data: []kraken.BalanceData{
					// GALA sorts before BTC here on purpose: Hydrate must
					// not pair whichever holding it reaches first with a
					// different symbol's trade.
					{Asset: "GALA", Balance: *decimal.NewFromFloat64(13536.853376037476)},
					{Asset: "BTC", Balance: *decimal.NewFromFloat64(0.0001)},
				},
			},
		}

		history := &kraken.TradesHistory{
			Result: kraken.TradesHistoryResult{
				Trades: map[string]spot.Trade{
					"gala-buy": {
						Pair:   "GALAUSD",
						Type:   "buy",
						Price:  decimal.NewFromFloat64(0.00232),
						Volume: decimal.NewFromFloat64(13536.853376037476),
					},
					"btc-buy": {
						OrderID: "btc-order",
						Pair:    "BTCUSD",
						Type:    "buy",
						Time:    decimal.NewFromInt64(1_700_000_000),
						Price:   decimal.NewFromFloat64(64129.900),
						Cost:    decimal.NewFromFloat64(6.41299),
						Fee:     decimal.NewFromFloat64(0.016673774),
						Volume:  decimal.NewFromFloat64(0.0001),
						TradeID: decimal.NewFromInt64(1),
					},
				},
			},
		}

		position := &Position{
			api:     &websocket.API{},
			ui:      ui,
			price:   price,
			balance: balance,
			thesis:  types.NewThesis(nil),
			Data:    &PositionData{},
		}

		position.Hydrate("BTC/USD", history)

		Convey("It should hydrate the quantity from BTC's own holding, not GALA's", func() {
			So(position.Data.Qty.Float64(), ShouldEqual, 0.0001)
			So(position.Data.EntryPrice.Float64(), ShouldEqual, 64129.900)
			So(position.Status(), ShouldEqual, types.OPEN)
		})

		Convey("It should value the position off BTC's own 0.0001 quantity, not GALA's", func() {
			So(position.Data.PnL.Float64(), ShouldAlmostEqual, -0.142114, 0.000001)
		})
	})
}

func TestPositionReconcile(t *testing.T) {
	Convey("Given a closed round trip followed by the wallet's current buy", t, func() {
		currentQuantity, err := decimal.NewFromString("1.0000000000004")
		So(err, ShouldBeNil)
		currentCost, err := decimal.NewFromString("120.000000000048")
		So(err, ShouldBeNil)
		position := &Position{
			balance: &Balance{quote: "USD"},
			Data:    &PositionData{Symbol: "BTC/USD"},
		}
		history := &kraken.TradesHistory{Result: kraken.TradesHistoryResult{
			Trades: map[string]spot.Trade{
				"closed-buy": {
					Pair: "BTCUSD", Type: "buy", Time: decimal.NewFromInt64(1),
					Price: decimal.NewFromInt64(100), Cost: decimal.NewFromInt64(100),
					Fee: decimal.NewFromInt64(1), Volume: decimal.NewFromInt64(1),
				},
				"closed-sell": {
					Pair: "BTCUSD", Type: "sell", Time: decimal.NewFromInt64(2),
					Price: decimal.NewFromInt64(110), Cost: decimal.NewFromInt64(110),
					Fee: decimal.NewFromInt64(1), Volume: decimal.NewFromInt64(1),
				},
				"current-buy": {
					Pair: "BTCUSD", Type: "buy", Time: decimal.NewFromInt64(3),
					Price: decimal.NewFromInt64(120), Cost: currentCost,
					Fee: decimal.NewFromInt64(1), Volume: currentQuantity,
				},
			},
		}}

		err = position.reconcile(history, "BTC/USD", SpotHolding{
			Asset: "BTC", Qty: *currentQuantity,
		})

		Convey("Then only the still-open chronological lot becomes the cost basis", func() {
			So(err, ShouldBeNil)
			So(position.Data.Qty.String(), ShouldEqual, "1.0000000000004")
			So(position.Data.EntryPrice.Float64(), ShouldEqual, 120.0)
			So(position.Executions(), ShouldHaveLength, 1)
			So(position.Executions()[0].Data[0].ExecID, ShouldEqual, "current-buy")
		})
	})
}

func BenchmarkPositionReconcile(b *testing.B) {
	history := &kraken.TradesHistory{Result: kraken.TradesHistoryResult{
		Trades: map[string]spot.Trade{
			"closed-buy": {
				Pair: "BTCUSD", Type: "buy", Time: decimal.NewFromInt64(1),
				Price: decimal.NewFromInt64(100), Cost: decimal.NewFromInt64(100),
				Fee: decimal.NewFromInt64(1), Volume: decimal.NewFromInt64(1),
			},
			"closed-sell": {
				Pair: "BTCUSD", Type: "sell", Time: decimal.NewFromInt64(2),
				Price: decimal.NewFromInt64(110), Cost: decimal.NewFromInt64(110),
				Fee: decimal.NewFromInt64(1), Volume: decimal.NewFromInt64(1),
			},
			"current-buy": {
				Pair: "BTCUSD", Type: "buy", Time: decimal.NewFromInt64(3),
				Price: decimal.NewFromInt64(120), Cost: decimal.NewFromInt64(120),
				Fee: decimal.NewFromInt64(1), Volume: decimal.NewFromInt64(1),
			},
		},
	}}
	holding := SpotHolding{Asset: "BTC", Qty: *decimal.NewFromInt64(1)}

	b.ReportAllocs()

	for b.Loop() {
		position := &Position{
			balance: &Balance{quote: "USD"},
			Data:    &PositionData{Symbol: "BTC/USD"},
		}

		if err := position.reconcile(history, "BTC/USD", holding); err != nil {
			b.Fatal(err)
		}
	}
}

/*
BenchmarkPositionTickerAck measures cached validation and symbol filtering on
the position ticker path.
*/
func BenchmarkPositionTickerAck(b *testing.B) {
	position := &Position{
		status: types.OPEN,
		Data: &PositionData{
			Symbol:     "NPC/USD",
			Qty:        *decimal.NewFromInt64(1),
			EntryPrice: *decimal.NewFromInt64(100),
		},
	}
	frame := []byte(`{
		"channel":"ticker",
		"type":"update",
		"data":[{"symbol":"BTC/USD","last":"110"}]
	}`)

	b.ReportAllocs()

	for b.Loop() {
		position.TickerAck(frame)
	}
}
