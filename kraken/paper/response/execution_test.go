package response

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

func TestExecutionsPublishFill(t *testing.T) {
	configurePaperWallet()

	Convey("Given wired paper orders and executions", t, func() {
		pool := qpool.NewQ(context.Background(), 1, 4, nil)
		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		sub := raw.Subscribe("test:executions", 32)
		ids := NewIdentifier()
		balances := NewBalances(raw, nil, ids)
		executions := NewExecutions(raw, balances, ids)
		orders := NewOrders(context.Background(), pool, balances, ids)
		orders.Observe(executions)

		Convey("A fill publishes Kraken executions frames and a derived trader envelope", func() {
			orders.notifyFill(FillNotice{
				Params: trading.AddParams{
					OrderType: trading.Limit,
					Side:      trading.Buy,
					Symbol:    "BTC/EUR",
					OrderQty:  0.001,
					ClOrdID:   "entry-1",
				},
				OrderID:      ids.OrderID(),
				Price:        100,
				Fee:          0.025,
				LiquidityInd: "m",
				Maker:        true,
			})

			var derived map[string]any
			var channelFrame map[string]any

			deadline := time.After(500 * time.Millisecond)

		collect:
			for {
				select {
				case msg := <-sub.Incoming:
					frame, ok := msg.Value.(map[string]any)

					if !ok {
						continue
					}

					if frame["channel"] == "executions" && frame["type"] == "update" {
						channelFrame = frame
					}

					if frame["channel"] == "executions" && frame["qty"] != nil {
						qty, _ := frame["qty"].(float64)

						if qty > 0 {
							derived = frame
						}
					}

					if channelFrame != nil && derived != nil {
						break collect
					}
				case <-deadline:
					break collect
				}
			}

			So(derived, ShouldNotBeNil)
			So(derived["symbol"], ShouldEqual, "BTC/EUR")
			So(derived["side"], ShouldEqual, "buy")
			So(derived["qty"], ShouldEqual, 0.001)
			So(channelFrame, ShouldNotBeNil)
			So(channelFrame["type"], ShouldEqual, "update")
		})
	})
}

func TestExecutionsSubscribe(t *testing.T) {
	Convey("Given an executions socket", t, func() {
		pool := qpool.NewQ(context.Background(), 1, 4, nil)
		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		sub := raw.Subscribe("test:executions:snap", 8)
		executions := NewExecutions(raw, nil, NewIdentifier())

		Convey("Subscribe publishes an empty executions snapshot", func() {
			ack := executions.Send(&qpool.QValue[any]{
				Value: user.ExecutionSubscribeFrame{
					Method: "subscribe",
					Params: user.ExecutionParams{Channel: "executions", SnapOrders: true, SnapTrades: true},
				},
			})

			So(ack["success"], ShouldBeTrue)

			select {
			case msg := <-sub.Incoming:
				frame, _ := msg.Value.(map[string]any)
				So(frame["channel"], ShouldEqual, "executions")
				So(frame["type"], ShouldEqual, "snapshot")
			case <-time.After(200 * time.Millisecond):
				So("snapshot timeout", ShouldBeEmpty)
			}
		})
	})
}
