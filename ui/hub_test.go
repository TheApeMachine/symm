package ui

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

func TestHubTickForwardsUIFrames(t *testing.T) {
	Convey("Given a connected frontend client", t, func() {
		viper.Set("system.queue.buffer", 64)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 2, 4, nil)

		hub := &Hub{
			ctx: ctx,
			bus: internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelUI},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelUI, "ui:test"),
				},
			),
		}

		server := httptest.NewServer(http.HandlerFunc(hub.handleWS))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

		conn, _, dialErr := websocket.DefaultDialer.Dial(wsURL, nil)

		So(dialErr, ShouldBeNil)
		So(conn, ShouldNotBeNil)

		defer conn.Close()

		_, helloPayload, readErr := conn.ReadMessage()

		So(readErr, ShouldBeNil)
		So(string(helloPayload), ShouldContainSubstring, `"event":"hello"`)

		go func() {
			_ = hub.Tick()
		}()

		sendErr := hub.bus.Send(internal.ChannelUI, "gauge", map[string]any{
			"chart":      "gauge",
			"source":     "cvd",
			"confidence": 0.75,
		})

		So(sendErr, ShouldBeNil)

		_ = conn.SetReadDeadline(time.Now().Add(time.Second))

		_, framePayload, frameErr := conn.ReadMessage()

		So(frameErr, ShouldBeNil)
		So(string(framePayload), ShouldContainSubstring, `"source":"cvd"`)
	})
}

func TestHubWriteRejectsNonFiniteJSON(t *testing.T) {
	Convey("Given a ui frame with non-finite floats", t, func() {
		hub := &Hub{}

		server := httptest.NewServer(http.HandlerFunc(hub.handleWS))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

		conn, _, dialErr := websocket.DefaultDialer.Dial(wsURL, nil)

		So(dialErr, ShouldBeNil)

		defer conn.Close()

		_, _, readErr := conn.ReadMessage()

		So(readErr, ShouldBeNil)

		hub.write(map[string]any{
			"type": "fluid",
			"re":   math.Inf(1),
		})

		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

		_, _, frameErr := conn.ReadMessage()

		Convey("It should fail JSON encoding before websocket write", func() {
			So(frameErr, ShouldNotBeNil)
		})
	})
}
