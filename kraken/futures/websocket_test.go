package futures

import (
	"context"
	"errors"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/internal/testconfig"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestWebSocketConnectReturnsCanceledContext(t *testing.T) {
	convey.Convey("Given a canceled futures websocket context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		websocketClient := &WebSocket{
			ctx: ctx,
		}

		convey.Convey("It should return without dialing or sleeping", func() {
			connectErr := websocketClient.Connect(0)

			convey.So(errors.Is(connectErr, context.Canceled), convey.ShouldBeTrue)
			convey.So(websocketClient.isConnected.Load(), convey.ShouldBeFalse)
		})
	})
}

func TestWebSocketDispatchBookSnapshot(t *testing.T) {
	convey.Convey("Given a futures book snapshot frame", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()

		websocketClient := &WebSocket{
			ctx:      ctx,
			registry: NewBookRegistry(),
			bus: internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelRaw},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelRaw, "test-futures-raw"),
				},
			),
		}

		payload := []byte(`{
			"feed":"book_snapshot",
			"product_id":"PI_XBTUSD",
			"timestamp":1612269825817,
			"seq":10,
			"tickSize":null,
			"bids":[{"price":100,"qty":5}],
			"asks":[{"price":101,"qty":4}]
		}`)

		convey.Convey("It should publish one raw book batch", func() {
			websocketClient.dispatch(payload)

			raw, err := websocketClient.bus.Receive(internal.ChannelRaw)
			convey.So(err, convey.ShouldBeNil)
			convey.So(raw, convey.ShouldNotBeNil)

			updates, ok := raw.Value.(*krakenmarket.BookUpdates)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(*updates), convey.ShouldEqual, 1)
			convey.So((*updates)[0].Symbol, convey.ShouldEqual, "PI_XBTUSD")
		})
	})
}

func TestNewWebSocketReturnsDistinctInstances(t *testing.T) {
	convey.Convey("Given repeated futures websocket construction", t, func() {
		testconfig.Load(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		defer pool.Close()

		firstClient := NewWebSocket(ctx, pool)
		secondClient := NewWebSocket(ctx, pool)

		defer func() {
			if firstClient != nil {
				convey.So(firstClient.Close(), convey.ShouldBeNil)
			}

			if secondClient != nil {
				convey.So(secondClient.Close(), convey.ShouldBeNil)
			}
		}()

		convey.Convey("It should isolate websocket state per instance", func() {
			convey.So(firstClient, convey.ShouldNotBeNil)
			convey.So(secondClient, convey.ShouldNotBeNil)
			convey.So(firstClient, convey.ShouldNotEqual, secondClient)
		})
	})
}
