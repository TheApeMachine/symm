package broker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

type recordingSocket struct {
	channels      map[string]chan []byte
	tickers       map[string]kraken.TickerData
	tickerSymbols [][]string
}

func (socket *recordingSocket) Observe(channel string) chan []byte {
	if socket.channels == nil {
		socket.channels = map[string]chan []byte{}
	}

	if socket.channels[channel] == nil {
		socket.channels[channel] = make(chan []byte, 8)
	}

	return socket.channels[channel]
}

func (socket *recordingSocket) Ticker(symbols []string) (kraken.TickerDataSlice, error) {
	socket.tickerSymbols = append(socket.tickerSymbols, append([]string(nil), symbols...))
	rows := make(kraken.TickerDataSlice, 0, len(symbols))

	for _, symbol := range symbols {
		if ticker, ok := socket.tickers[symbol]; ok {
			rows = append(rows, ticker)
			continue
		}

		rows = append(rows, kraken.TickerData{
			Symbol: symbol,
			Bid:    *decimal.NewFromFloat64(102),
			Ask:    *decimal.NewFromFloat64(102.1),
			Last:   *decimal.NewFromFloat64(102),
		})
	}

	return rows, nil
}

type recordingPrivate struct {
	recordingSocket
	orders []*kraken.Order
}

func (private *recordingPrivate) Submit(order *kraken.Order) error {
	private.orders = append(private.orders, order)
	return nil
}

func (private *recordingPrivate) TradeVolume(_ []string) (websocket.FeeSchedule, error) {
	return websocket.FeeSchedule{
		Fallback: websocket.FeeRates{Taker: 0.0026, Maker: 0.0016},
		Pairs:    map[string]websocket.FeeRates{},
	}, nil
}

func (private *recordingPrivate) Close() {
}

func testDecimal(testingTB testing.TB, value string) decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		testingTB.Fatal(err)
	}

	return *parsed
}

func TestDeskRun(testingTB *testing.T) {
	Convey("Given a desk with canonical private streams", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &recordingSocket{}
		private := &recordingPrivate{}
		ui := make(chan []byte, 8)
		desk, err := NewDesk(ctx, public, private, ui)

		So(err, ShouldBeNil)
		position := NewPosition(private, &PositionData{
			Symbol: "NEAR/USD",
			Qty:    10,
		})
		position.orderID = "client-77"
		desk.positions.Store("NEAR/USD", position)

		Convey("When account snapshots and an add_order response arrive", func() {
			done := make(chan error, 1)
			go func() {
				done <- desk.Run()
			}()

			private.channels[channelBalances] <- []byte(`[{
				"asset": "USD",
				"asset_class": "currency",
				"balance": 200.18,
				"available": 180
			}]`)
			private.channels[channelAddOrder] <- []byte(`{
				"method": "add_order",
				"result": {
					"cl_ord_id": "client-77",
					"order_id": "O-NEAR-1"
				},
				"success": true,
				"req_id": 77
			}`)
			private.channels[channelOrders] <- []byte(`[{
				"id": "O-NEAR-1",
				"pair": "NEAR/USD",
				"price": 1.8,
				"reserved_amount": 18,
				"reserved_asset": "USD",
				"side": "buy",
				"type": "limit",
				"volume": 10
			}]`)
			private.channels[channelExecutions] <- []byte(`[{
				"exec_id": "T-NEAR-1",
				"order_id": "O-NEAR-1",
				"symbol": "NEAR/USD",
				"side": "buy",
				"order_status": "filled",
				"last_qty": 10
			}]`)

			time.Sleep(10 * time.Millisecond)
			cancel()

			Convey("Then the position should keep each canonical record", func() {
				So(<-done, ShouldBeNil)
				So(desk.balance, ShouldNotBeNil)
				So((*desk.balance)[0].Asset, ShouldEqual, "USD")
				So(position.order.ID, ShouldEqual, "O-NEAR-1")
				So(position.executions, ShouldHaveLength, 1)
				So(position.executions[0].ExecID, ShouldEqual, "T-NEAR-1")
			})

			Convey("Then the desk should forward canonical UI batches", func() {
				orders := []map[string]any{}
				executions := []map[string]any{}
				deadline := time.After(time.Second)

				for len(orders) == 0 || len(executions) == 0 {
					select {
					case wire := <-ui:
						frame := map[string]json.RawMessage{}
						So(sonic.Unmarshal(wire, &frame), ShouldBeNil)

						if rows, ok := frame[channelOrders]; ok {
							orders = []map[string]any{}
							So(sonic.Unmarshal(rows, &orders), ShouldBeNil)
						}

						if rows, ok := frame[channelExecutions]; ok {
							executions = []map[string]any{}
							So(sonic.Unmarshal(rows, &executions), ShouldBeNil)
						}
					case <-deadline:
						testingTB.Fatal("desk did not forward UI batches")
					}
				}

				So(orders[0]["id"], ShouldEqual, "O-NEAR-1")
				So(executions[0]["exec_id"], ShouldEqual, "T-NEAR-1")
			})
		})
	})
}

func TestDeskBuy(testingTB *testing.T) {
	Convey("Given a desk with a private submitter", testingTB, func() {
		previousNormalSlots := viper.GetInt("trading.slots.normal")
		previousReservedSlots := viper.GetInt("trading.slots.reserved")
		viper.Set("trading.slots.normal", 1)
		viper.Set("trading.slots.reserved", 0)
		defer viper.Set("trading.slots.normal", previousNormalSlots)
		defer viper.Set("trading.slots.reserved", previousReservedSlots)

		public := &recordingSocket{}
		private := &recordingPrivate{}
		ui := make(chan []byte, 8)
		desk, err := NewDesk(context.Background(), public, private, ui)

		So(err, ShouldBeNil)

		Convey("When Buy is called before account hydration", func() {
			price := testDecimal(testingTB, "100000.00000000")
			err := desk.Buy("BTC/USD", 0.05, price, false)

			Convey("Then it should remain idle", func() {
				So(err, ShouldNotBeNil)
				So(private.orders, ShouldHaveLength, 0)
			})
		})

		Convey("When Buy is called after account hydration", func() {
			desk.balance = &kraken.BalanceDataSlice{{
				Asset:     "USD",
				Available: testDecimal(testingTB, "200.00000000"),
				Balance:   testDecimal(testingTB, "200.00000000"),
			}}
			price := testDecimal(testingTB, "100000.00000000")
			err := desk.Buy("BTC/USD", 0.05, price, false)

			Convey("Then it should size and submit a Kraken order", func() {
				So(err, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 1)
				position, ok := desk.positions.Load("BTC/USD")
				So(ok, ShouldBeTrue)
				So(position.(*Position).data.Mark.String(), ShouldEqual, "100000.00000000")
				So(private.orders, ShouldHaveLength, 1)
				So(private.orders[0].Method, ShouldEqual, "add_order")
				params := private.orders[0].Params.(kraken.LimitOrderParams)
				So(params.OrderQty, ShouldAlmostEqual, 0.0001)
			})
		})

		Convey("When Buy sizes an integer-scale quote balance against a sub-unit price", func() {
			desk.balance = &kraken.BalanceDataSlice{{
				Asset:     "USD",
				Available: *decimal.NewFromFloat64(200),
				Balance:   *decimal.NewFromFloat64(200),
			}}
			price := testDecimal(testingTB, "0.25000000")
			err := desk.Buy("HONEY/USD", 0.05, price, false)

			Convey("Then it should size and submit without rounding the price denominator to zero", func() {
				So(err, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 1)
				So(private.orders, ShouldHaveLength, 1)
				params := private.orders[0].Params.(kraken.LimitOrderParams)
				So(params.OrderQty, ShouldAlmostEqual, 40)
			})
		})
	})
}

func TestDeskSetFeeSchedule(testingTB *testing.T) {
	Convey("Given a desk with restored open positions", testingTB, func() {
		desk := &Desk{
			positions:       &sync.Map{},
			feeSchedule:     &sync.Map{},
			fallbackFeeRate: 0.001,
		}
		night := NewPosition(nil, &PositionData{Symbol: "NIGHT/USD"})
		mana := NewPosition(nil, &PositionData{Symbol: "MANA/USD"})
		desk.positions.Store("NIGHT/USD", night)
		desk.positions.Store("MANA/USD", mana)

		Convey("When the trade-volume schedule arrives after restoration", func() {
			err := desk.SetFeeSchedule(websocket.FeeSchedule{
				Fallback: websocket.FeeRates{Taker: 0.0026},
				Pairs: map[string]websocket.FeeRates{
					"MANA/USD": {Taker: 0.0018},
				},
			})

			Convey("Then existing positions receive pair or account-tier fees", func() {
				So(err, ShouldBeNil)
				So(night.data.FeeRate, ShouldEqual, 0.0026)
				So(mana.data.FeeRate, ShouldEqual, 0.0018)
			})
		})
	})
}

func BenchmarkDeskBuy(benchmarkTB *testing.B) {
	previousNormalSlots := viper.GetInt("trading.slots.normal")
	previousReservedSlots := viper.GetInt("trading.slots.reserved")
	viper.Set("trading.slots.normal", 1)
	viper.Set("trading.slots.reserved", 0)
	defer viper.Set("trading.slots.normal", previousNormalSlots)
	defer viper.Set("trading.slots.reserved", previousReservedSlots)

	public := &recordingSocket{}
	private := &recordingPrivate{}
	desk, err := NewDesk(context.Background(), public, private, make(chan []byte, 8))
	if err != nil {
		benchmarkTB.Fatal(err)
	}

	desk.feeSchedule.Store("HONEY/USD", websocket.FeeRates{Taker: 0.0026})
	desk.balance = &kraken.BalanceDataSlice{{
		Asset:     "USD",
		Available: *decimal.NewFromFloat64(200),
		Balance:   *decimal.NewFromFloat64(200),
	}}
	price := testDecimal(benchmarkTB, "0.25000000")

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		desk.positions = &sync.Map{}
		private.orders = private.orders[:0]

		if err := desk.Buy("HONEY/USD", 0.05, price, false); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}

func seedOpenPosition(private *recordingPrivate, symbol string) *Position {
	position := NewPosition(private, &PositionData{
		Symbol: symbol,
		Qty:    1.0,
	})
	position.status = types.OPEN

	return position
}

func TestDeskSellAlwaysExecutes(testingTB *testing.T) {
	Convey("Given a desk holding an open position", testingTB, func() {
		// Sell must never be gated on capacity status: a close has to fire in
		// every state so a full book can always reclaim a slot by exiting.
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
					params := private.orders[0].Params.(kraken.LimitOrderParams)
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

func TestDeskPositions(testingTB *testing.T) {
	Convey("Given a desk with open positions", testingTB, func() {
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

func TestDeskRunReconcilesExecutionSnapshots(testingTB *testing.T) {
	Convey("Given a running desk with local positions", testingTB, func() {
		Convey("When an execution snapshot restores an open position", func() {
			public := &recordingSocket{
				tickers: map[string]kraken.TickerData{
					"SPACE/USD": {
						Symbol: "SPACE/USD",
						Bid:    *decimal.NewFromFloat64(0.0064),
						Ask:    *decimal.NewFromFloat64(0.0065),
						Last:   *decimal.NewFromFloat64(0.0064),
					},
				},
			}
			private := &recordingPrivate{}
			desk := &Desk{
				public:          public,
				private:         private,
				positions:       &sync.Map{},
				feeSchedule:     &sync.Map{},
				fallbackFeeRate: 0,
				maxPositions:    4,
			}
			desk.feeSchedule.Store("SPACE/USD", websocket.FeeRates{})

			desk.Executions(&kraken.ExecutionDataSlice{{
				AvgPrice:       *decimal.NewFromFloat64(0.006),
				ExecType:       "snapshot",
				LastQty:        100,
				OrderStatus:    "filled",
				PositionStatus: "open",
				Side:           "buy",
				Symbol:         "SPACE/USD",
			}})

			Convey("Then the restored position is marked from REST ticker data", func() {
				position, ok := desk.positions.Load("SPACE/USD")
				So(ok, ShouldBeTrue)
				So(public.tickerSymbols, ShouldResemble, [][]string{{"SPACE/USD"}})
				So(position.(*Position).data.Mark.String(), ShouldEqual, "0.0064")
				So(position.(*Position).data.PnL.String(), ShouldEqual, "0.0400")
			})
		})

		Convey("When a complete execution snapshot omits a closing position", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			public := &recordingSocket{}
			private := &recordingPrivate{}
			ui := make(chan []byte, 8)
			desk, err := NewDesk(ctx, public, private, ui)
			So(err, ShouldBeNil)

			desk.positions.Store("ETH/USD", seedOpenPosition(private, "ETH/USD"))
			done := make(chan error, 1)
			go func() {
				done <- desk.Run()
			}()

			err = desk.Sell("ETH/USD")
			So(err, ShouldBeNil)
			private.channels[channelExecutions] <- []byte(`[]`)

			deadline := time.After(time.Second)
			for desk.OpenPositions() != 0 {
				select {
				case <-ui:
				case <-deadline:
					testingTB.Fatal("closing position was not reaped from execution snapshot")
				default:
					time.Sleep(time.Millisecond)
				}
			}

			cancel()

			Convey("Then the stale local position is removed and the slot is freed", func() {
				So(<-done, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 0)
			})
		})

		Convey("When a complete execution snapshot omits a pending buy", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			public := &recordingSocket{}
			private := &recordingPrivate{}
			ui := make(chan []byte, 8)
			desk, err := NewDesk(ctx, public, private, ui)
			So(err, ShouldBeNil)

			position := seedOpenPosition(private, "SOL/USD")
			position.status = types.PENDING
			position.closing = false
			desk.positions.Store("SOL/USD", position)
			done := make(chan error, 1)
			go func() {
				done <- desk.Run()
			}()

			private.channels[channelExecutions] <- []byte(`[]`)

			select {
			case <-ui:
			case <-time.After(time.Second):
				testingTB.Fatal("desk did not process execution snapshot")
			}

			cancel()

			Convey("Then the pending entry is not confused with a confirmed close", func() {
				So(<-done, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 1)
			})
		})
	})
}

func TestDeskRefreshStatus(testingTB *testing.T) {
	Convey("Given a desk with one normal slot and one reserved slot", testingTB, func() {
		newDesk := func() *Desk {
			return &Desk{
				status:       types.INITIALIZING,
				positions:    &sync.Map{},
				maxPositions: 1,
				maxReserved:  1,
			}
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

func TestDeskBuyRejectedWhenBusy(testingTB *testing.T) {
	Convey("Given a hydrated desk that is BUSY", testingTB, func() {
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

func TestDeskBuyRejectedBeforeHydration(testingTB *testing.T) {
	Convey("Given a READY desk whose account has not hydrated", testingTB, func() {
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

func BenchmarkDeskOpenPositions(benchmarkTB *testing.B) {
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

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = desk.OpenPositions()
	}
}

func BenchmarkDeskPositions(benchmarkTB *testing.B) {
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

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = desk.Positions()
	}
}

func BenchmarkDeskExecutions(benchmarkTB *testing.B) {
	private := &recordingPrivate{}
	executions := &kraken.ExecutionDataSlice{{
		AvgPrice:       *decimal.NewFromFloat64(101),
		ExecType:       "snapshot",
		LastQty:        2,
		OrderStatus:    "filled",
		PositionStatus: "open",
		Side:           "buy",
		Symbol:         "ETH/USD",
	}}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		desk := &Desk{
			public:          &recordingSocket{},
			private:         private,
			positions:       &sync.Map{},
			feeSchedule:     &sync.Map{},
			fallbackFeeRate: 0.0026,
			maxPositions:    4,
		}
		desk.feeSchedule.Store("ETH/USD", websocket.FeeRates{Taker: 0.0026})
		desk.positions.Store("ETH/USD", seedOpenPosition(private, "ETH/USD"))
		desk.positions.Store("STALE/USD", seedOpenPosition(private, "STALE/USD"))

		desk.Executions(executions)
	}
}
