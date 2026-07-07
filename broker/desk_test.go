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

	. "github.com/smartystreets/goconvey/convey"
)

type recordingSocket struct {
	channels map[string]chan []byte
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

type recordingPrivate struct {
	recordingSocket
	orders []*kraken.Order
}

func (private *recordingPrivate) Submit(order *kraken.Order) error {
	private.orders = append(private.orders, order)
	return nil
}

func (private *recordingPrivate) Close() {
}

func testDecimal(testingTB *testing.T, value string) decimal.Decimal {
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
		previousMaxPositions := viper.GetInt("trading.max_concurrent_positions")
		viper.Set("trading.max_concurrent_positions", 1)
		defer viper.Set("trading.max_concurrent_positions", previousMaxPositions)

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
				So(private.orders, ShouldHaveLength, 1)
				So(private.orders[0].Method, ShouldEqual, "add_order")
				params := private.orders[0].Params.(kraken.LimitOrderParams)
				So(params.OrderQty, ShouldAlmostEqual, 0.0001)
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

func BenchmarkDeskReady(benchmarkTB *testing.B) {
	desk := &Desk{
		balance: &kraken.BalanceDataSlice{{
			Asset:     "USD",
			Available: *decimal.NewFromFloat64(200),
			Balance:   *decimal.NewFromFloat64(200),
		}},
	}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = desk.Ready()
	}
}
