package public

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestEnsureOhlcSubscriptionRequiresOutbound(t *testing.T) {
	Convey("Given a public websocket without kraken:public wired", t, func() {
		ws := &WebSocket{
			broadcasts:     make(map[string]*qpool.BroadcastGroup),
			ohlcSubscribed: make(map[string]struct{}),
		}

		ws.ensureOhlcSubscription("BTC/EUR")

		Convey("It should not mark the symbol subscribed", func() {
			_, known := ws.ohlcSubscribed["BTC/EUR"]
			So(known, ShouldBeFalse)
		})
	})

	Convey("Given kraken:public outbound is ready", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := &WebSocket{
			broadcasts:     make(map[string]*qpool.BroadcastGroup),
			ohlcSubscribed: make(map[string]struct{}),
		}

		outbound := pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
		ws.broadcasts["kraken:public"] = outbound
		subscriber := outbound.Subscribe("test:public:ohlc", 4)

		ws.ensureOhlcSubscription("BTC/EUR")

		Convey("It should enqueue an ohlc subscribe frame", func() {
			_, known := ws.ohlcSubscribed["BTC/EUR"]
			So(known, ShouldBeTrue)

			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)
				So(ok, ShouldBeTrue)
				So(payload["method"], ShouldEqual, "subscribe")

				params, ok := payload["params"].(map[string]any)

				So(ok, ShouldBeTrue)
				So(params["channel"], ShouldEqual, CandlesChannel)
			case <-time.After(500 * time.Millisecond):
				So("ohlc subscribe frame", ShouldBeBlank)
			}
		})
	})
}

func TestResubscribeOhlc(t *testing.T) {
	Convey("Given a stale subscription marker", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := &WebSocket{
			broadcasts:     make(map[string]*qpool.BroadcastGroup),
			ohlcSubscribed: map[string]struct{}{"BTC/EUR": {}},
		}

		outbound := pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond)
		ws.broadcasts["kraken:public"] = outbound
		subscriber := outbound.Subscribe("test:public:resubscribe", 4)

		ws.resubscribeOhlc("BTC/EUR")

		Convey("It should publish a fresh ohlc subscribe frame", func() {
			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)
				So(ok, ShouldBeTrue)
				So(payload["method"], ShouldEqual, "subscribe")
			case <-time.After(500 * time.Millisecond):
				So("ohlc resubscribe frame", ShouldBeBlank)
			}
		})
	})
}
