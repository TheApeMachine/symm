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
			So(results.Which(), ShouldEqual, Received_Which_frame)

			readData, err := results.Frame().Read()
			So(err, ShouldBeNil)
			So(string(readData), ShouldEqual, "echo:hello")
			endpoint, err := results.Frame().Endpoint()
			So(err, ShouldBeNil)
			So(endpoint, ShouldEqual, wsURL)
			receivedAt, err := results.Frame().ReceivedAt()
			So(err, ShouldBeNil)
			instant, err := time.Parse(time.RFC3339Nano, receivedAt)
			So(err, ShouldBeNil)
			So(instant.IsZero(), ShouldBeFalse)
			So(instant.After(time.Now()), ShouldBeFalse)

			client.Close()
		})
	})
}

func TestWebSocketClientDone(t *testing.T) {
	Convey("Given a disconnected source with accepted frames", t, func() {
		ctx := context.Background()
		server := NewWebSocketClient(ctx)
		server.Transition(runtime.WAITING)
		client := WebSocketClient_ServerToClient(server)
		defer client.Release()
		instant := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
		server.incoming.Enqueue(receivedFrame{payload: []byte("first"), at: instant, endpoint: "ws://capture"})
		server.incoming.Enqueue(receivedFrame{payload: []byte("second"), at: instant.Add(time.Second), endpoint: "ws://capture"})

		Convey("Then Done drains in order regardless of connection status and becomes idle", func() {
			for index, expected := range []string{"first", "second"} {
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Which(), ShouldEqual, Received_Which_frame)
				payload, err := result.Frame().Read()
				So(err, ShouldBeNil)
				So(string(payload), ShouldEqual, expected)
				receivedAt, err := result.Frame().ReceivedAt()
				So(err, ShouldBeNil)
				So(receivedAt, ShouldEqual, instant.Add(time.Duration(index)*time.Second).Format(time.RFC3339Nano))
				release()
			}

			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, Received_Which_idle)
		})
	})
}
