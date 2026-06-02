package public

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestApplyOhlc(t *testing.T) {
	Convey("Given a websocket with chart streams", t, func() {
		testconfig.Load(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()
		streams := focus.NewSet()
		streams.Add("BTC/EUR")

		webSocket := &WebSocket{
			broadcasts: map[string]*qpool.BroadcastGroup{
				"ui:charts": pool.CreateBroadcastGroup("ui:charts", 10*time.Millisecond),
			},
			streams: streams,
		}

		subscriber := webSocket.broadcasts["ui:charts"].Subscribe("test:charts", 4)

		message := SocketMessage{
			Channel: CandlesChannel,
			Type:    "update",
			Data: []byte(`[{
				"symbol":"BTC/EUR",
				"open":1,
				"high":2,
				"low":0.5,
				"close":1.5,
				"volume":12.5,
				"interval_begin":"2023-10-04T16:25:00.000000000Z",
				"interval":1
			}]`),
		}

		Convey("It should publish candle_bar frames", func() {
			So(webSocket.applyOhlc(message), ShouldBeNil)

			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)

				So(ok, ShouldBeTrue)
				So(payload["event"], ShouldEqual, "candle_bar")
				So(payload["symbol"], ShouldEqual, "BTC/EUR")
			case <-time.After(time.Second):
				So("candle_bar frame", ShouldBeBlank)
			}
		})
	})
}

func TestBindChartStreams(t *testing.T) {
	Convey("Given a focus set with an anchor symbol configured", t, func() {
		testconfig.Load(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		webSocket := &WebSocket{
			broadcasts: map[string]*qpool.BroadcastGroup{
				"kraken:public": pool.CreateBroadcastGroup("kraken:public", 10*time.Millisecond),
			},
			ohlcSubscribed: make(map[string]struct{}),
		}

		streams := focus.NewSet()
		streams.Add("ETH/EUR")

		Convey("It should register stream notifications", func() {
			webSocket.bindChartStreams(streams)

			So(webSocket.streams, ShouldEqual, streams)
		})
	})
}
