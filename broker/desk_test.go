package broker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
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
		socket.channels[channel] = make(chan []byte, 4)
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

func TestDeskRun(testingTB *testing.T) {
	Convey("Given a desk with public and private byte streams", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &recordingSocket{}
		private := &recordingPrivate{}
		desk, err := NewDesk(ctx, public, private)

		So(err, ShouldBeNil)
		desk.UIForward = make(chan []byte, 4)

		Convey("When balance and execution payloads arrive", func() {
			done := make(chan error, 1)
			go func() {
				done <- desk.Run()
			}()

			private.channels[channelBalances] <- []byte(`[{
				"asset": "USD",
				"asset_class": "currency",
				"balance": 200.18
			}]`)
			private.channels[channelExecutions] <- []byte(`[{
				"exec_id": "PAPER-00002",
				"order_id": "PAPER-00002",
				"symbol": "NEAR/USD",
				"side": "sell",
				"order_status": "filled",
				"order_qty": 20,
				"fee": 0.104
			}]`)

			time.Sleep(10 * time.Millisecond)
			cancel()

			Convey("Then the desk should retain the decoded state", func() {
				So(<-done, ShouldBeNil)
				So(desk.balance, ShouldNotBeNil)
				So(*desk.balance, ShouldHaveLength, 1)
				So((*desk.balance)[0].Asset, ShouldEqual, "USD")
				So(desk.executions, ShouldHaveLength, 1)
				So((*desk.executions[0])[0].ExecID, ShouldEqual, "PAPER-00002")
			})

			Convey("Then the desk should forward named UI batches", func() {
				frames := map[string][]map[string]any{}
				deadline := time.After(time.Second)

				for len(frames) < 2 {
					select {
					case wire := <-desk.UIForward:
						frame := map[string][]map[string]any{}
						So(sonic.Unmarshal(wire, &frame), ShouldBeNil)

						for key, rows := range frame {
							frames[key] = rows
						}
					case <-deadline:
						testingTB.Fatal("desk did not forward named UI batches")
					}
				}

				So(frames[channelBalances], ShouldHaveLength, 1)
				So(frames[channelBalances][0]["asset"], ShouldEqual, "USD")
				So(frames[channelExecutions], ShouldHaveLength, 1)
				So(frames[channelExecutions][0]["exec_id"], ShouldEqual, "PAPER-00002")
			})
		})
	})
}

func TestDeskOpenPositions(testingTB *testing.T) {
	Convey("Given a desk that opens positions through buy orders", testingTB, func() {
		public := &recordingSocket{}
		private := &recordingPrivate{}
		desk, err := NewDesk(context.Background(), public, private)

		So(err, ShouldBeNil)
		balance := kraken.BalanceDataSlice{{
			Asset:     "USD",
			Available: 200,
			Balance:   200,
		}, {
			Asset:     "EUR",
			Available: 500,
			Balance:   500,
		}}
		desk.balance = &balance

		So(desk.OpenPositions(), ShouldEqual, 0)

		Convey("When buys are submitted", func() {
			firstErr := desk.Buy("BTC/USD", 0.05, 100000)
			secondErr := desk.Buy("SOL/USD", 0.10, 20)

			Convey("Then only tracked positions count as open", func() {
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 2)
				So(private.orders, ShouldHaveLength, 2)
			})
		})

		Convey("When a tracked symbol is sold", func() {
			So(desk.Buy("BTC/USD", 0.05, 100000), ShouldBeNil)
			So(desk.Buy("SOL/USD", 0.10, 20), ShouldBeNil)
			sellErr := desk.Sell("BTC/USD")

			Convey("Then the matching position is removed", func() {
				So(sellErr, ShouldBeNil)
				So(desk.OpenPositions(), ShouldEqual, 1)
				_, btcOpen := desk.positions.Load("BTC/USD")
				solPositions, solOpen := desk.positions.Load("SOL/USD")
				So(btcOpen, ShouldBeFalse)
				So(solOpen, ShouldBeTrue)
				So(solPositions.([]*Position), ShouldHaveLength, 1)
			})
		})
	})
}

func TestDeskBuy(testingTB *testing.T) {
	Convey("Given a desk with a private submitter", testingTB, func() {
		public := &recordingSocket{}
		private := &recordingPrivate{}
		desk, err := NewDesk(context.Background(), public, private)

		So(err, ShouldBeNil)
		balance := kraken.BalanceDataSlice{{
			Asset:     "USD",
			Available: 200,
			Balance:   200,
		}}
		desk.balance = &balance

		Convey("When Buy submits a market order", func() {
			err := desk.Buy("BTC/USD", 0.05, 100000)

			Convey("Then it should submit a Kraken order", func() {
				So(err, ShouldBeNil)
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
	desk.positions.Store("BTC/USD", []*Position{{Symbol: "BTC/USD", Qty: 0.01}})
	desk.positions.Store("SOL/USD", []*Position{{Symbol: "SOL/USD", Qty: 3.5}})

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = desk.OpenPositions()
	}
}
