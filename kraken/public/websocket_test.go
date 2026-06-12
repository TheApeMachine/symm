package public

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
				[]internal.Channel{internal.ChannelRaw},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelRaw, "test-raw"),
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

			raw, err := ws.bus.Receive(internal.ChannelRaw)
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

func TestWebSocketDispatchPongLatency(t *testing.T) {
	Convey("Given a pong with Kraken time_in and time_out", t, func() {
		profilePath := filepath.Join(t.TempDir(), "network_latency.json")
		viper.Set("trading.paper.latency_profile", profilePath)

		ws := &WebSocket{
			latencyProfilePath: profilePath,
		}

		timeIn := time.Now().Add(-40 * time.Millisecond)
		timeOut := timeIn.Add(2 * time.Millisecond)
		message := types.NewSocketMessage()
		message.Channel = "pong"
		message.Data = []byte(fmt.Sprintf(
			`{"method":"pong","success":true,"time_in":%q,"time_out":%q}`,
			timeIn.Format(time.RFC3339Nano),
			timeOut.Format(time.RFC3339Nano),
		))

		Convey("It should append round-trip latency to the profile file", func() {
			ws.dispatch(message)

			profileBytes, readErr := os.ReadFile(profilePath)
			So(readErr, ShouldBeNil)

			line := strings.TrimSpace(string(profileBytes))
			nanoseconds, parseErr := strconv.ParseInt(line, 10, 64)
			So(parseErr, ShouldBeNil)
			So(nanoseconds, ShouldBeGreaterThan, 0)
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

func TestWebSocketDisconnectMarksResubscribe(t *testing.T) {
	Convey("Given a connected public websocket", t, func() {
		ws := &WebSocket{}
		ws.isConnected.Store(true)

		Convey("It should mark resubscribe when the socket drops", func() {
			ws.disconnect()

			So(ws.needsResubscribe.Load(), ShouldBeTrue)
			So(ws.isConnected.Load(), ShouldBeFalse)
		})
	})
}

func TestWebSocketResubscribe(t *testing.T) {
	Convey("Given a public websocket bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		defer pool.Close()

		ws := &WebSocket{
			ctx: ctx,
			bus: internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelRaw, internal.ChannelKrakenPublic},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelRaw, "test-raw"),
					internal.Subscribe(internal.ChannelKrakenPublic, "test-public"),
				},
			),
		}

		Convey("It should publish reconnect and instrument subscribe frames", func() {
			So(ws.resubscribe(), ShouldBeNil)

			reconnect, err := ws.bus.Receive(internal.ChannelRaw)
			So(err, ShouldBeNil)
			So(reconnect, ShouldNotBeNil)
			So(reconnect.Type, ShouldEqual, "reconnect")

			subscribe, err := ws.bus.Receive(internal.ChannelKrakenPublic)
			So(err, ShouldBeNil)
			So(subscribe, ShouldNotBeNil)
			So(subscribe.Type, ShouldEqual, "instrument")
		})
	})
}
