package broker

import (
	"context"
	"testing"
	"time"

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
