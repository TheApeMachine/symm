package ui

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	wswebsocket "github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestHubWebSocketReplaysLatestUIArtifact(testingTB *testing.T) {
	Convey("Given a hub with a current backend UI artifact", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)

		listenAddr := listener.Addr().String()
		So(listener.Close(), ShouldBeNil)

		previousAddr := viper.GetString("ui.addr")
		viper.Set("ui.addr", listenAddr)
		defer viper.Set("ui.addr", previousAddr)

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		hub, hubErr := NewHub(ctx, pool, dmt.NewTree(""))
		So(hubErr, ShouldBeNil)

		payload := []byte(`{"count":7,"phase":"stream"}`)
		artifact := datura.Acquire("trader", datura.APPJSON).
			WithRole("tick").
			WithScope("tick").
			WithDestination("ui").
			WithPayload(payload)

		So(hub.snapshot.Observe(artifact), ShouldBeNil)

		serverErrors := serveHub(testingTB, hub)

		defer func() {
			So(hub.Close(), ShouldBeNil)

			select {
			case err := <-serverErrors:
				if err != nil && !strings.Contains(err.Error(), "server is not running") {
					testingTB.Errorf("hub run failed: %v", err)
				}
			case <-time.After(time.Second):
				testingTB.Errorf("hub did not stop")
			}
		}()

		var conn *wswebsocket.Conn

		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			conn, _, err = wswebsocket.DefaultDialer.Dial("ws://"+listenAddr+"/ws", nil)

			if err == nil {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		So(err, ShouldBeNil)
		defer conn.Close()
		waitHubClient(testingTB, hub)

		So(conn.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)

		messageType, wire, err := conn.ReadMessage()

		So(err, ShouldBeNil)
		So(messageType, ShouldEqual, wswebsocket.BinaryMessage)

		var received datura.Artifact
		_, err = received.Unpack(wire)

		So(err, ShouldBeNil)

		role, err := received.Role()

		So(err, ShouldBeNil)
		So(role, ShouldEqual, "tick")
		So(string(received.DecryptPayload()), ShouldEqual, string(payload))
	})
}
