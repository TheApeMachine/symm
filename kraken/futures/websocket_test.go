package futures

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

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
