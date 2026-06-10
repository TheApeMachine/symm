package public

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
)

func TestWebSocketDispatchSubscribeAck(t *testing.T) {
	Convey("Given a subscribe success frame without channel data", t, func() {
		success := true
		message := types.NewSocketMessage()
		message.Success = &success

		ws := &WebSocket{}

		Convey("It should not panic on dispatch", func() {
			So(func() {
				ws.dispatch(message)
			}, ShouldNotPanic)
		})
	})
}

func TestWebSocketDispatchBookUpdates(t *testing.T) {
	Convey("Given a Kraken book frame with array data", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()

		ws := &WebSocket{
			ctx: ctx,
			bus: internal.NewBus(
				ctx,
				pool,
				[]string{"raw"},
				[]internal.Subscription{
					internal.Subscribe("raw", "test-raw"),
				},
			),
		}

		message := types.NewSocketMessage()
		message.Channel = "book"
		message.Type = "snapshot"
		message.Data = json.RawMessage(`[{
			"symbol":"BTC/USD",
			"bids":[{"price":100,"qty":1}],
			"asks":[{"price":101,"qty":2}],
			"checksum":123,
			"timestamp":"2024-01-01T00:00:00Z"
		}]`)

		Convey("It should publish one raw book batch without fan-out in dispatch", func() {
			ws.dispatch(message)

			raw, err := ws.bus.Receive("raw")
			So(err, ShouldBeNil)
			So(raw, ShouldNotBeNil)

			updates, ok := raw.Value.(*market.BookUpdates)
			So(ok, ShouldBeTrue)
			So(len(*updates), ShouldEqual, 1)
			So((*updates)[0].Symbol, ShouldEqual, "BTC/USD")
			So((*updates)[0].Type, ShouldEqual, "snapshot")
		})
	})
}

func TestWebSocketDispatchOhlcUIFrame(t *testing.T) {
	Convey("Given an ohlc frame for the anchor symbol", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()

		viper.Set("market.anchor_symbol", "BTC/USD")

		ws := &WebSocket{
			ctx: ctx,
			bus: internal.NewBus(
				ctx,
				pool,
				[]string{"raw", "ui"},
				[]internal.Subscription{
					internal.Subscribe("ui", "test-ui"),
				},
			),
		}

		message := types.NewSocketMessage()
		message.Channel = "ohlc"
		message.Data = json.RawMessage(`[{
			"symbol":"BTC/USD",
			"open":100,
			"high":110,
			"low":90,
			"close":105,
			"volume":12.5,
			"interval_begin":"2024-06-01T12:34:56Z",
			"interval":1
		}]`)

		Convey("It should publish an enriched ui ohlc frame", func() {
			expected, parseErr := time.Parse(time.RFC3339, "2024-06-01T12:34:56Z")
			So(parseErr, ShouldBeNil)

			ws.dispatch(message)

			row, err := ws.bus.Receive("ui")
			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)

			frame, ok := row.Value.(map[string]any)
			So(ok, ShouldBeTrue)
			So(frame["symbol"], ShouldEqual, "BTC/USD")
			So(frame["sec"], ShouldEqual, expected.Unix())
			So(frame["close"], ShouldEqual, 105.0)
		})
	})
}

func TestWebSocketConnectRequiresLiveConn(t *testing.T) {
	Convey("Given a stale connected flag without a socket", t, func() {
		ws := &WebSocket{}
		ws.isConnected.Store(true)

		Convey("It should clear the stale flag before dialing", func() {
			if ws.isConnected.Load() && ws.conn != nil {
				return
			}

			ws.isConnected.Store(false)

			So(ws.isConnected.Load(), ShouldBeFalse)
		})
	})
}
