package websocket

import (
	"context"
	"encoding/json"
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
				frames, err := params.NewWrite(1)
				if err != nil {
					return err
				}
				return frames.Set(0, []byte("hello"))
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
		server.incoming.Enqueue(receivedFrame{payload: []byte("first"), at: instant, endpoint: "ws://capture", sequence: 91})
		server.incoming.Enqueue(receivedFrame{payload: []byte("second"), at: instant.Add(time.Second), endpoint: "ws://capture", sequence: 92})

		Convey("Then Done drains in order regardless of connection status and becomes idle", func() {
			for index, expected := range []string{"first", "second"} {
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				So(result.Which(), ShouldEqual, Received_Which_frame)
				payload, err := result.Frame().Read()
				So(err, ShouldBeNil)
				So(string(payload), ShouldEqual, expected)
				provenance, err := result.Frame().Provenance()
				So(err, ShouldBeNil)
				var origin struct {
					Session  string
					Sequence int64
					Endpoint string
				}
				So(json.Unmarshal(provenance, &origin), ShouldBeNil)
				So(origin.Session, ShouldEqual, server.session)
				So(origin.Sequence, ShouldEqual, 91+index)
				So(origin.Endpoint, ShouldEqual, "ws://capture")
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

func TestWebSocketClientWrite(t *testing.T) {
	Convey("Given a graph-declared connection message and multiple outbound frames", t, func() {
		received := make(chan string, 16)
		failures := make(chan error, 16)
		disconnected := make(chan struct{}, 1)
		upgrader := gorillaws.Upgrader{}
		venue := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				failures <- err
				return
			}
			defer func() {
				if err := connection.Close(); err != nil {
					failures <- err
				}
			}()
			for {
				_, payload, err := connection.ReadMessage()
				if err != nil {
					return
				}
				received <- string(payload)
				if string(payload) == "disconnect" {
					disconnected <- struct{}{}
					return
				}
			}
		}))
		defer venue.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server := NewWebSocketClient(ctx)
		client := WebSocketClient_ServerToClient(server)
		defer client.Release()
		write := func(payloads ...string) {
			So(client.Write(ctx, func(params WebSocketClient_write_Params) error {
				if err := params.SetEndpoint("ws" + strings.TrimPrefix(venue.URL, "http")); err != nil {
					return err
				}
				handshake, err := params.NewOnConnect(2)

				if err != nil {
					return err
				}

				if err := handshake.Set(0, []byte("ticker")); err != nil {
					return err
				}

				if err := handshake.Set(1, []byte("trade")); err != nil {
					return err
				}
				frames, err := params.NewWrite(int32(len(payloads)))
				if err != nil {
					return err
				}
				for index, payload := range payloads {
					if err := frames.Set(index, []byte(payload)); err != nil {
						return err
					}
				}
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}
		next := func(expected string) {
			select {
			case payload := <-received:
				So(payload, ShouldEqual, expected)
			case err := <-failures:
				So(err, ShouldBeNil)
			case <-ctx.Done():
				t.Fatal("timed out waiting for " + expected)
			}
		}
		write()
		next("ticker")
		next("trade")
		write("ping")
		next("ping")
		write("disconnect")
		next("disconnect")
		<-disconnected

		next("ticker")
		next("trade")
		So(server.Close(), ShouldBeNil)
	})
}
