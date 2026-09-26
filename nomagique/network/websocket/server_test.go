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

func TestWebSocketServerBroadcast(t *testing.T) {
	Convey("A browser that stops reading cannot block the broadcasting node", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server := NewWebSocketServer(ctx)
		joined := make(chan struct{}, 1)
		defer func() { So(server.Close(), ShouldBeNil) }()
		handler := server.UpgradeHandler()
		venue := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			handler(writer, request)
			joined <- struct{}{}
		}))
		defer venue.Close()
		slow, _, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(venue.URL, "http"), nil)
		So(err, ShouldBeNil)
		<-joined
		defer func() { So(slow.Close(), ShouldBeNil) }()
		// A frame exceeds typical socket buffering so a non-reading peer
		// leaves its writer occupied; the transport permits one pending frame.
		payload := make([]byte, 4<<20)
		finished := make(chan struct{})

		go func() {
			defer close(finished)

			for range 4 {
				server.Broadcast(payload)
			}
		}()

		select {
		case <-finished:
		case <-ctx.Done():
			t.Fatal("a slow browser blocked broadcast")
		}
		remaining := 0
		server.clients.Range(func(key, value any) bool { remaining++; return true })
		So(remaining, ShouldEqual, 0)

		Convey("A reading peer still receives an immutable frame afterwards", func() {
			healthy, _, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(venue.URL, "http"), nil)
			So(err, ShouldBeNil)
			<-joined
			defer func() { So(healthy.Close(), ShouldBeNil) }()
			So(healthy.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
			frame := []byte("completed metric cut")
			server.Broadcast(frame)
			copy(frame, "mutated after send!!")
			_, received, err := healthy.ReadMessage()
			So(err, ShouldBeNil)
			So(string(received), ShouldEqual, "completed metric cut")
		})
	})
}

func BenchmarkWebSocketServerBroadcast(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewWebSocketServer(ctx)
	joined := make(chan struct{}, 1)
	defer func() {
		if err := server.Close(); err != nil {
			b.Error(err)
		}
	}()
	handler := server.UpgradeHandler()
	venue := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handler(writer, request)
		joined <- struct{}{}
	}))
	defer venue.Close()
	connection, _, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(venue.URL, "http"), nil)

	if err != nil {
		b.Fatal(err)
	}
	<-joined
	defer func() {
		if err := connection.Close(); err != nil {
			b.Error(err)
		}
	}()
	// Representative serialized metric payload; count matches the signal graph.
	payload := []byte(strings.Repeat(`{"value":0.125,"present":true},`, 411))
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()

	for b.Loop() {
		server.Broadcast(payload)
		_, received, err := connection.ReadMessage()

		if err != nil {
			b.Fatal(err)
		}

		if len(received) != len(payload) {
			b.Fatal("broadcast payload was truncated")
		}
	}
}

func TestWebSocketServerShutdown(t *testing.T) {
	Convey("The capability lifetime owns the socket pumps", t, func() {
		server := NewWebSocketServer(context.Background())
		client := WebSocketServer_ServerToClient(server)
		client.Release()

		select {
		case <-server.Context().Done():
		case <-time.After(time.Second):
			t.Fatal("releasing the server capability did not close its transport")
		}
	})
}
