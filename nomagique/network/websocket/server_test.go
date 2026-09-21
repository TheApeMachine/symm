package websocket

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestWebSocketServer(t *testing.T) {
	Convey("Given a WebSocketServerServer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewWebSocketServer(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capServer := WebSocketServer_ServerToClient(server)
		So(capServer.IsValid(), ShouldBeTrue)

		testHTTPServer := httptest.NewServer(server.UpgradeHandler())
		defer testHTTPServer.Close()

		wsURL := "ws://" + strings.TrimPrefix(testHTTPServer.URL, "http://")

		Convey("Client connects, sends message, and server broadcasts back", func() {
			dialer := gorillaws.Dialer{}
			conn, _, err := dialer.Dial(wsURL, nil)
			So(err, ShouldBeNil)
			defer conn.Close()

			// Send message from client to server
			err = conn.WriteMessage(gorillaws.TextMessage, []byte(`{"type":"FOCUS","symbol":"BTC/USD"}`))
			So(err, ShouldBeNil)

			// Wait briefly for read pump
			time.Sleep(50 * time.Millisecond)

			// Check focus channel
			select {
			case sym := <-server.Focus():
				So(sym, ShouldEqual, "BTC/USD")
			case <-time.After(500 * time.Millisecond):
				t.Fatal("timed out waiting for focus symbol")
			}

			// Verify server Done yields incoming message
			future, release := capServer.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Status(), ShouldEqual, runtime.Status_ready)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldContainSubstring, "BTC/USD")

			// Write broadcast from server to client
			err = capServer.Write(ctx, func(params WebSocketServer_write_Params) error {
				return params.SetData([]byte("broadcast-payload"))
			})
			So(err, ShouldBeNil)

			_, clientReceived, err := conn.ReadMessage()
			So(err, ShouldBeNil)
			So(string(clientReceived), ShouldEqual, "broadcast-payload")

			_ = server.Close()
		})
	})
}
