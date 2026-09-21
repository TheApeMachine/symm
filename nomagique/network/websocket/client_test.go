package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestWebSocketClient(t *testing.T) {
	upgrader := gorillaws.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(messageType, append([]byte("echo:"), data...)); err != nil {
				return
			}
		}
	}))
	defer mockServer.Close()

	wsURL := "ws://" + strings.TrimPrefix(mockServer.URL, "http://")

	Convey("Given a WebSocketClientServer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := NewWebSocketClient(ctx)
		So(client, ShouldNotBeNil)

		capClient := WebSocketClient_ServerToClient(client)
		So(capClient.IsValid(), ShouldBeTrue)

		Convey("Connecting and exchanging frames", func() {
			err := capClient.Write(ctx, func(params WebSocketClient_write_Params) error {
				if err := params.SetEndpoint(wsURL); err != nil {
					return err
				}
				return params.SetWrite([]byte("hello"))
			})
			So(err, ShouldBeNil)

			// Wait briefly for connection and echo response
			time.Sleep(150 * time.Millisecond)

			So(client.Status(), ShouldEqual, runtime.READY)

			future, release := capClient.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Status(), ShouldEqual, runtime.Status_ready)

			readData, err := results.Read()
			So(err, ShouldBeNil)
			So(string(readData), ShouldEqual, "echo:hello")

			client.Close()
		})
	})
}
