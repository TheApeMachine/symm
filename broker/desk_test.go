package broker

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type recordingPrivate struct {
	orders []*kraken.Order
}

func (private *recordingPrivate) Client() *spot.WebSocket {
	return nil
}

func (private *recordingPrivate) On(string, func([]byte)) {}

func (private *recordingPrivate) Write(params json.Marshaler) error {
	body, err := params.MarshalJSON()

	if err != nil {
		return err
	}

	order := &kraken.Order{}

	if sonic.Unmarshal(body, order) != nil {
		return nil
	}

	private.orders = append(private.orders, order)

	return nil
}

func (private *recordingPrivate) Get(string, json.Marshaler) ([]byte, error) {
	return nil, nil
}

func (private *recordingPrivate) Post(string, json.Marshaler) ([]byte, error) {
	return nil, nil
}

func (private *recordingPrivate) Close() {}

var _ websocket.Conn = (*recordingPrivate)(nil)

func TestBalancesOn(t *testing.T) {
	Convey("Given a desk with a position", t, func() {
		private := &recordingPrivate{}
		ui := make(chan []byte, 8)
		desk := NewDesk(private, private, nil)
		balances := NewBalances(desk, ui)
		position := NewPosition(private, &PositionData{
			Symbol: "NEAR/USD",
			Qty:    10,
		})
		position.orderID = "client-77"
		desk.positions.Store("NEAR/USD", position)

		Convey("When balances arrive", func() {
			balances.On([]byte(`[{
				"asset": "USD",
				"asset_class": "currency",
				"balance": 200.18,
				"available": 180
			}]`))

			Convey("Then the account hydrates and publishes balances", func() {
				So(desk.balance, ShouldNotBeNil)
				So((*desk.balance)[0].Asset, ShouldEqual, "USD")

				frame := map[string]json.RawMessage{}
				So(sonic.Unmarshal(<-ui, &frame), ShouldBeNil)
				So(frame["balances"], ShouldNotBeEmpty)
			})
		})
	})
}

func TestOrdersAck(t *testing.T) {
	Convey("Given a desk with a pending position", t, func() {
		private := &recordingPrivate{}
		desk := NewDesk(private, private, nil)
		orders := NewOrders(desk, nil)
		position := NewPosition(private, &PositionData{
			Symbol: "NEAR/USD",
			Qty:    10,
		})
		position.orderID = "client-77"
		desk.positions.Store("NEAR/USD", position)

		Convey("When an add_order response arrives", func() {
			orders.Ack([]byte(`{
				"method": "add_order",
				"result": {
					"cl_ord_id": "client-77",
					"order_id": "O-NEAR-1"
				},
				"success": true,
				"req_id": 77
			}`))

			Convey("Then the position is acknowledged", func() {
				So(position.status, ShouldEqual, types.OPEN)
			})
		})
	})
}

func TestOrdersOn(t *testing.T) {
	Convey("Given a desk with an open position", t, func() {
		private := &recordingPrivate{}
		desk := NewDesk(private, private, nil)
		orders := NewOrders(desk, nil)
		position := NewPosition(private, &PositionData{
			Symbol: "NEAR/USD",
			Qty:    10,
		})
		desk.positions.Store("NEAR/USD", position)

		Convey("When an order update arrives", func() {
			orders.On([]byte(`[{
				"id": "O-NEAR-1",
				"pair": "NEAR/USD",
				"price": 1.8,
				"reserved_amount": 18,
				"reserved_asset": "USD",
				"side": "buy",
				"type": "limit",
				"volume": 10
			}]`))

			Convey("Then the position keeps the order", func() {
				So(position.order.ID, ShouldEqual, "O-NEAR-1")
			})
		})
	})
}

func TestExecutionsOn(t *testing.T) {
	Convey("Given a desk with an open position", t, func() {
		private := &recordingPrivate{}
		ui := make(chan []byte, 8)
		desk := NewDesk(private, private, nil)
		executions := NewExecutions(desk, ui)
		position := NewPosition(private, &PositionData{
			Symbol: "NEAR/USD",
			Qty:    10,
		})
		desk.positions.Store("NEAR/USD", position)

		Convey("When an execution arrives", func() {
			executions.On([]byte(`[{
				"exec_id": "T-NEAR-1",
				"order_id": "O-NEAR-1",
				"symbol": "NEAR/USD",
				"side": "buy",
				"order_status": "filled",
				"last_qty": 10
			}]`))

			Convey("Then the position keeps the execution and publishes UI batches", func() {
				So(position.executions, ShouldHaveLength, 1)
				So(position.executions[0].ExecID, ShouldEqual, "T-NEAR-1")

				frame := map[string]json.RawMessage{}
				So(sonic.Unmarshal(<-ui, &frame), ShouldBeNil)

				batches := [][]map[string]any{}
				So(sonic.Unmarshal(frame["executions"], &batches), ShouldBeNil)
				So(len(batches), ShouldEqual, 1)
				So(len(batches[0]), ShouldBeGreaterThan, 0)
				So(batches[0][0]["exec_id"], ShouldEqual, "T-NEAR-1")
			})
		})

		Convey("When an execution snapshot restores an open position", func() {
			desk := &Desk{
				private:         private,
				positions:       &sync.Map{},
				feeSchedule:     &sync.Map{},
				fallbackFeeRate: 0,
				maxPositions:    4,
			}
			desk.feeSchedule.Store("SPACE/USD", kraken.FeeRates{})
			snapshotExecutions := NewExecutions(desk, nil)
			snapshotMark := NewMark(desk, nil)

			snapshotExecutions.On([]byte(`[{
				"symbol": "SPACE/USD",
				"avg_price": 0.006,
				"exec_type": "snapshot",
				"last_qty": 100,
				"order_status": "filled",
				"position_status": "open",
				"side": "buy"
			}]`))

			snapshotMark.On([]byte(`[{
				"symbol": "SPACE/USD",
				"bid": 0.0064,
				"ask": 0.0065,
				"last": 0.0064
			}]`))

			Convey("Then the restored position is marked from ticker data", func() {
				position, ok := desk.positions.Load("SPACE/USD")
				So(ok, ShouldBeTrue)
				So(position.(*Position).data.Mark.String(), ShouldEqual, "0.0064")
				So(position.(*Position).data.PnL.String(), ShouldEqual, "0.0400")
			})
		})

		Convey("When a complete execution snapshot omits a closing position", func() {
			desk := NewDesk(private, private, nil)
			desk.positions.Store("ETH/USD", seedOpenPosition(private, "ETH/USD"))

			So(desk.Sell("ETH/USD"), ShouldBeNil)
			NewExecutions(desk, nil).On([]byte(`[]`))

			Convey("Then the stale local position is removed and the slot is freed", func() {
				So(desk.OpenPositions(), ShouldEqual, 0)
			})
		})

		Convey("When a complete execution snapshot omits a pending buy", func() {
			desk := NewDesk(private, private, nil)
			position := seedOpenPosition(private, "SOL/USD")
			position.status = types.PENDING
			position.closing = false
			desk.positions.Store("SOL/USD", position)

			NewExecutions(desk, nil).On([]byte(`[]`))

			Convey("Then the pending entry is not confused with a confirmed close", func() {
				So(desk.OpenPositions(), ShouldEqual, 1)
			})
		})
	})
}

func TestMarkOn(t *testing.T) {
	Convey("Given a desk holding an open position", t, func() {
		private := &recordingPrivate{}
		desk := NewDesk(private, private, nil)
		mark := NewMark(desk, nil)
		position := seedOpenPosition(private, "BTC/USD")
		position.SetFeeRate(0.0026)
		position.data.EntryPrice = tests.Decimal(t, "100")
		desk.positions.Store("BTC/USD", position)

		Convey("When ticker data arrives", func() {
			mark.On([]byte(`[{
				"symbol": "BTC/USD",
				"bid": 101,
				"ask": 102,
				"last": 101.5
			}]`))

			Convey("Then the position mark updates", func() {
				So(position.data.Mark.String(), ShouldEqual, "101")
			})
		})
	})
}

func TestDeskBuy(t *testing.T) {
	Convey("Given a desk with a private submitter", t, func() {
		previousNormalSlots := viper.GetInt("trading.slots.normal")
		previousReservedSlots := viper.GetInt("trading.slots.reserved")
		viper.Set("trading.slots.normal", 1)
		viper.Set("trading.slots.reserved", 0)
		defer viper.Set("trading.slots.normal", previousNormalSlots)
		defer viper.Set("trading.slots.reserved", previousReservedSlots)

		private := &recordingPrivate{}
		desk := NewDesk(private, private, nil)

		Convey("When Buy is called before account hydration", func() {
			price := tests.Decimal(t, "100000.00000000")
			err := desk.Buy("BTC/USD", 0.05, price, false)

			Convey("Then it should remain idle", func() {
				So(err, ShouldNotBeNil)
				So(private.orders, ShouldHaveLength, 0)
			})
		})

		Convey("When Buy is called after account hydration", func() {
			desk.balance = &kraken.BalanceDataSlice{{
				Asset:     "USD",
				Available: tests.Decimal(t, "200.00000000"),
				Balance:   tests.Decimal(t, "200.00000000"),
			}}
			price := tests.Decimal(t, "100000.00000000")
			err := desk.Buy("BTC/USD", 0.05, price, false)

			Convey("Then it should size and submit a Kraken order", func() {
				So(err, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 1)
				position, ok := desk.positions.Load("BTC/USD")
				So(ok, ShouldBeTrue)
				So(position.(*Position).data.Mark.String(), ShouldEqual, "100000.00000000")
				So(private.orders, ShouldHaveLength, 1)
				So(private.orders[0].Method, ShouldEqual, "add_order")
				paramsBody, marshalErr := sonic.Marshal(private.orders[0].Params)
				So(marshalErr, ShouldBeNil)
				params := kraken.LimitOrderParams{}
				So(sonic.Unmarshal(paramsBody, &params), ShouldBeNil)
				So(params.OrderQty, ShouldAlmostEqual, 0.0001)
			})
		})

		Convey("When Buy sizes an integer-scale quote balance against a sub-unit price", func() {
			desk.balance = &kraken.BalanceDataSlice{{
				Asset:     "USD",
				Available: *decimal.NewFromFloat64(200),
				Balance:   *decimal.NewFromFloat64(200),
			}}
			price := tests.Decimal(t, "0.25000000")
			err := desk.Buy("HONEY/USD", 0.05, price, false)

			Convey("Then it should size and submit without rounding the price denominator to zero", func() {
				So(err, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 1)
				So(private.orders, ShouldHaveLength, 1)
				paramsBody, marshalErr := sonic.Marshal(private.orders[0].Params)
				So(marshalErr, ShouldBeNil)
				params := kraken.LimitOrderParams{}
				So(sonic.Unmarshal(paramsBody, &params), ShouldBeNil)
				So(params.OrderQty, ShouldAlmostEqual, 40)
			})
		})
	})
}

func TestDeskTakerRate(t *testing.T) {
	Convey("Given a desk with pair and fallback fee rates", t, func() {
		desk := &Desk{
			feeSchedule:     &sync.Map{},
			fallbackFeeRate: 0.0026,
		}
		desk.feeSchedule.Store("MANA/USD", kraken.FeeRates{Taker: 0.0018})

		Convey("Then pair rates override the fallback", func() {
			So(desk.takerRate("MANA/USD"), ShouldEqual, 0.0018)
			So(desk.takerRate("NIGHT/USD"), ShouldEqual, 0.0026)
		})
	})
}

func TestDeskSell(t *testing.T) {
	Convey("Given a desk holding an open position", t, func() {
		for _, status := range []types.Status{
			types.READY, types.PRIORITY, types.BUSY, types.INITIALIZING,
		} {
			Convey("When Sell is called while the desk status is "+string(status), func() {
				private := &recordingPrivate{}
				desk := &Desk{
					status:    status,
					positions: &sync.Map{},
				}
				desk.positions.Store("ETH/USD", seedOpenPosition(private, "ETH/USD"))

				err := desk.Sell("ETH/USD")

				Convey("Then the exit order is submitted regardless of status", func() {
					So(err, ShouldBeNil)
					So(private.orders, ShouldHaveLength, 1)
					So(private.orders[0].Method, ShouldEqual, "add_order")
					paramsBody, marshalErr := sonic.Marshal(private.orders[0].Params)
					So(marshalErr, ShouldBeNil)
					params := kraken.LimitOrderParams{}
					So(sonic.Unmarshal(paramsBody, &params), ShouldBeNil)
					So(params.Side, ShouldEqual, "sell")
					So(params.Symbol, ShouldEqual, "ETH/USD")
				})
			})
		}

		Convey("When Sell is called for a symbol that is not held", func() {
			desk := &Desk{status: types.BUSY, positions: &sync.Map{}}

			err := desk.Sell("NOPE/USD")

			Convey("Then it reports the position is not found", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestDeskPositions(t *testing.T) {
	Convey("Given a desk with open positions", t, func() {
		private := &recordingPrivate{}
		desk := &Desk{positions: &sync.Map{}}
		btc := seedOpenPosition(private, "BTC/USD")
		eth := seedOpenPosition(private, "ETH/USD")
		desk.positions.Store("ETH/USD", eth)
		desk.positions.Store("BTC/USD", btc)

		Convey("When Positions is called", func() {
			positions := desk.Positions()
			seen := map[*Position]bool{}

			for _, position := range positions {
				seen[position] = true
			}

			Convey("Then it returns live position pointers", func() {
				So(positions, ShouldHaveLength, 2)
				So(seen[btc], ShouldBeTrue)
				So(seen[eth], ShouldBeTrue)
			})
		})
	})
}

func TestDeskRefreshStatus(t *testing.T) {
	Convey("Given a desk with one normal slot and one reserved slot", t, func() {
		newDesk := func() *Desk {
			desk := &Desk{
				status:       types.INITIALIZING,
				positions:    &sync.Map{},
				maxPositions: 1,
				maxReserved:  1,
			}
			desk.balance = &kraken.BalanceDataSlice{{
				Asset: "USD",
			}}

			return desk
		}

		Convey("When no positions are open", func() {
			desk := newDesk()
			desk.refreshStatus()

			Convey("Then the desk is READY", func() {
				So(desk.Status(), ShouldEqual, types.READY)
			})
		})

		Convey("When the normal slots are full but a reserved slot remains", func() {
			desk := newDesk()
			desk.positions.Store("ETH/USD", seedOpenPosition(&recordingPrivate{}, "ETH/USD"))
			desk.refreshStatus()

			Convey("Then the desk is PRIORITY", func() {
				So(desk.Status(), ShouldEqual, types.PRIORITY)
			})
		})

		Convey("When both normal and reserved slots are full", func() {
			desk := newDesk()
			desk.positions.Store("ETH/USD", seedOpenPosition(&recordingPrivate{}, "ETH/USD"))
			desk.positions.Store("BTC/USD", seedOpenPosition(&recordingPrivate{}, "BTC/USD"))
			desk.refreshStatus()

			Convey("Then the desk is BUSY", func() {
				So(desk.Status(), ShouldEqual, types.BUSY)
			})
		})

		Convey("When a full book frees a slot", func() {
			desk := newDesk()
			desk.positions.Store("ETH/USD", seedOpenPosition(&recordingPrivate{}, "ETH/USD"))
			desk.positions.Store("BTC/USD", seedOpenPosition(&recordingPrivate{}, "BTC/USD"))
			desk.refreshStatus()
			So(desk.Status(), ShouldEqual, types.BUSY)

			desk.positions.Delete("BTC/USD")
			desk.refreshStatus()

			Convey("Then the desk recovers from BUSY down to PRIORITY", func() {
				So(desk.Status(), ShouldEqual, types.PRIORITY)
			})
		})
	})
}

func TestDeskBuyRejectedWhenBusy(t *testing.T) {
	Convey("Given a hydrated desk that is BUSY", t, func() {
		desk := &Desk{
			status:       types.BUSY,
			positions:    &sync.Map{},
			maxPositions: 1,
			maxReserved:  0,
			balance: &kraken.BalanceDataSlice{{
				Asset:     "USD",
				Available: *decimal.NewFromFloat64(200),
				Balance:   *decimal.NewFromFloat64(200),
			}},
		}

		Convey("When an opportunity Buy is attempted", func() {
			err := desk.Buy("ETH/USD", 0.05, *decimal.NewFromFloat64(100), true)

			Convey("Then even a high-value opportunity is refused", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestDeskBuyRejectedBeforeHydration(t *testing.T) {
	Convey("Given a READY desk whose account has not hydrated", t, func() {
		desk := &Desk{
			status:       types.READY,
			positions:    &sync.Map{},
			maxPositions: 4,
			maxReserved:  0,
		}

		Convey("When a Buy is attempted with no balance snapshot", func() {
			err := desk.Buy("ETH/USD", 0.05, *decimal.NewFromFloat64(100), false)

			Convey("Then it is refused without panicking on the nil balance", func() {
				So(err, ShouldNotBeNil)
				So(desk.OpenPositions(), ShouldEqual, 0)
			})
		})
	})
}

func seedOpenPosition(private *recordingPrivate, symbol string) *Position {
	position := NewPosition(private, &PositionData{
		Symbol: symbol,
		Qty:    1.0,
	})
	position.status = types.OPEN

	return position
}

func BenchmarkDeskBuy(b *testing.B) {
	previousNormalSlots := viper.GetInt("trading.slots.normal")
	previousReservedSlots := viper.GetInt("trading.slots.reserved")
	viper.Set("trading.slots.normal", 1)
	viper.Set("trading.slots.reserved", 0)
	defer viper.Set("trading.slots.normal", previousNormalSlots)
	defer viper.Set("trading.slots.reserved", previousReservedSlots)

	private := &recordingPrivate{}
	desk := NewDesk(private, private, nil)
	desk.feeSchedule.Store("HONEY/USD", kraken.FeeRates{Taker: 0.0026})
	desk.balance = &kraken.BalanceDataSlice{{
		Asset:     "USD",
		Available: *decimal.NewFromFloat64(200),
		Balance:   *decimal.NewFromFloat64(200),
	}}
	price := tests.Decimal(b, "0.25000000")

	b.ReportAllocs()
	for b.Loop() {
		desk.positions = &sync.Map{}
		private.orders = private.orders[:0]

		if err := desk.Buy("HONEY/USD", 0.05, price, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDeskOpenPositions(b *testing.B) {
	desk := &Desk{
		positions: &sync.Map{},
	}
	desk.positions.Store("BTC/USD", NewPosition(nil, &PositionData{
		Symbol: "BTC/USD",
		Qty:    0.01,
	}))
	desk.positions.Store("SOL/USD", NewPosition(nil, &PositionData{
		Symbol: "SOL/USD",
		Qty:    3.5,
	}))

	b.ReportAllocs()
	for b.Loop() {
		_ = desk.OpenPositions()
	}
}

func BenchmarkDeskPositions(b *testing.B) {
	desk := &Desk{
		positions: &sync.Map{},
	}
	desk.positions.Store("BTC/USD", NewPosition(nil, &PositionData{
		Symbol: "BTC/USD",
		Qty:    0.01,
	}))
	desk.positions.Store("SOL/USD", NewPosition(nil, &PositionData{
		Symbol: "SOL/USD",
		Qty:    3.5,
	}))

	b.ReportAllocs()
	for b.Loop() {
		_ = desk.Positions()
	}
}

func BenchmarkExecutionsMeasure(b *testing.B) {
	private := &recordingPrivate{}
	executionsSlice := &kraken.ExecutionDataSlice{{
		AvgPrice:       *decimal.NewFromFloat64(101),
		ExecType:       "snapshot",
		LastQty:        2,
		OrderStatus:    "filled",
		PositionStatus: "open",
		Side:           "buy",
		Symbol:         "ETH/USD",
	}}

	b.ReportAllocs()
	for b.Loop() {
		desk := &Desk{
			private:         private,
			positions:       &sync.Map{},
			feeSchedule:     &sync.Map{},
			fallbackFeeRate: 0.0026,
			maxPositions:    4,
		}
		desk.feeSchedule.Store("ETH/USD", kraken.FeeRates{Taker: 0.0026})
		desk.positions.Store("ETH/USD", seedOpenPosition(private, "ETH/USD"))
		desk.positions.Store("STALE/USD", seedOpenPosition(private, "STALE/USD"))

		NewExecutions(desk, nil).Measure("snapshot", executionsSlice)
	}
}
