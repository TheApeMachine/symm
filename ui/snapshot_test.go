package ui

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	wswebsocket "github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestSnapshotReplay(t *testing.T) {
	Convey("Given a hub with a current typed tick message", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		listenAddr := testListenAddr(t)
		previousAddr := viper.GetString("ui.addr")
		previousBuffer := viper.GetInt("system.websocket.channel.buffer")
		viper.Set("ui.addr", listenAddr)
		viper.Set("system.websocket.channel.buffer", 8)
		defer viper.Set("ui.addr", previousAddr)
		defer viper.Set("system.websocket.channel.buffer", previousBuffer)

		hub, err := NewHub(ctx)
		So(err, ShouldBeNil)
		So(hub.snapshot.Observe(Message{Tick: &Tick{Count: 11, Phase: "stream"}}), ShouldBeNil)
		serverErrors := serveHub(t, hub)
		defer closeHub(t, hub, serverErrors)

		Convey("When a websocket client connects", func() {
			conn := dialHub(t, listenAddr)
			defer conn.Close()
			So(conn.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
			messageType, wire, err := conn.ReadMessage()
			decoded := Message{}
			decodeErr := sonic.Unmarshal(wire, &decoded)

			Convey("Then it receives the latest typed snapshot message", func() {
				So(err, ShouldBeNil)
				So(messageType, ShouldEqual, wswebsocket.TextMessage)
				So(decodeErr, ShouldBeNil)
				So(decoded.Tick, ShouldNotBeNil)
				So(decoded.Tick.Count, ShouldEqual, 11)
			})
		})
	})
}

func TestSnapshotObserve(t *testing.T) {
	Convey("Given a dashboard snapshot", t, func() {
		snapshot := NewSnapshot()

		Convey("When it observes a decision event", func() {
			err := snapshot.Observe(Message{
				Decision: &Decision{
					Symbol:  "BTC/USD",
					Verdict: "allow",
				},
			})

			Convey("Then it does not replay stale decisions as current state", func() {
				seen := false
				snapshot.messages.Range(func(_, _ any) bool {
					seen = true
					return false
				})

				So(err, ShouldBeNil)
				So(seen, ShouldBeFalse)
			})
		})
	})
}
