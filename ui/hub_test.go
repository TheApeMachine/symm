package ui

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	wswebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestHubPublish(t *testing.T) {
	Convey("Given a hub websocket client", t, func() {
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
		serverErrors := serveHub(t, hub)
		defer closeHub(t, hub, serverErrors)

		conn := dialHub(t, listenAddr)
		defer conn.Close()
		waitHubClient(t, hub)

		Convey("When the backend forwards a named UI frame", func() {
			hub.Messages <- []byte(`{"tick":{"count":7,"phase":"stream"}}`)
			So(conn.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
			messageType, wire, err := conn.ReadMessage()
			decoded := map[string]any{}
			decodeErr := sonic.Unmarshal(wire, &decoded)

			Convey("Then the websocket receives the same UI frame", func() {
				So(err, ShouldBeNil)
				So(messageType, ShouldEqual, wswebsocket.TextMessage)
				So(decodeErr, ShouldBeNil)
				tick := decoded["tick"].(map[string]any)
				So(tick["count"], ShouldEqual, 7.0)
				So(tick["phase"], ShouldEqual, "stream")
			})
		})
	})
}

func TestHubReplayInstruments(t *testing.T) {
	Convey("Given a hub with a live websocket client", t, func() {
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
		serverErrors := serveHub(t, hub)
		defer closeHub(t, hub, serverErrors)

		first := dialHub(t, listenAddr)
		defer first.Close()
		waitHubClient(t, hub)

		Convey("When the backend forwards instruments before another client connects", func() {
			wire := []byte(`{"instruments":{"data":{"pairs":[{"symbol":"BTC/USD"}]}}}`)
			hub.Messages <- wire
			So(first.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
			_, _, err := first.ReadMessage()
			So(err, ShouldBeNil)

			second := dialHub(t, listenAddr)
			defer second.Close()
			So(second.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)
			messageType, replay, err := second.ReadMessage()
			decoded := map[string]any{}
			decodeErr := sonic.Unmarshal(replay, &decoded)

			Convey("Then the new client receives the latest instrument frame", func() {
				So(err, ShouldBeNil)
				So(messageType, ShouldEqual, wswebsocket.TextMessage)
				So(decodeErr, ShouldBeNil)
				So(decoded["instruments"], ShouldNotBeNil)
			})
		})
	})
}

func TestHubContextCancellation(t *testing.T) {
	Convey("Given a hub served from a cancellable context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		listenAddr := testListenAddr(t)
		previousAddr := viper.GetString("ui.addr")
		previousBuffer := viper.GetInt("system.websocket.channel.buffer")
		viper.Set("ui.addr", listenAddr)
		viper.Set("system.websocket.channel.buffer", 8)
		defer viper.Set("ui.addr", previousAddr)
		defer viper.Set("system.websocket.channel.buffer", previousBuffer)

		hub, err := NewHub(ctx)
		So(err, ShouldBeNil)
		serverErrors := serveHub(t, hub)
		conn := dialHub(t, listenAddr)
		So(conn.Close(), ShouldBeNil)

		Convey("When the backend context is canceled", func() {
			cancel()

			Convey("Then the websocket server stops instead of serving a stale snapshot", func() {
				select {
				case err := <-serverErrors:
					So(err == nil || strings.Contains(err.Error(), "server is not running"), ShouldBeTrue)
				case <-time.After(time.Second):
					t.Fatal("hub did not stop after context cancellation")
				}
			})
		})
	})
}

func testListenAddr(t testing.TB) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	listenAddr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	return listenAddr
}

func dialHub(t testing.TB, listenAddr string) *wswebsocket.Conn {
	t.Helper()

	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		conn, _, err := wswebsocket.DefaultDialer.Dial("ws://"+listenAddr+"/ws", nil)
		if err == nil {
			return conn
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("hub websocket did not accept connection")
	return nil
}

func waitHubClient(t testing.TB, hub *Hub) {
	t.Helper()

	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		found := false
		hub.clients.Range(func(_, _ any) bool {
			found = true
			return false
		})

		if found {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("hub websocket client was not registered")
}

func serveHub(t testing.TB, hub *Hub) chan error {
	t.Helper()

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- hub.app.Listen(hub.listenAddr, fiber.ListenConfig{
			EnablePrefork: false,
		})
	}()

	return serverErrors
}

func closeHub(t testing.TB, hub *Hub, serverErrors chan error) {
	t.Helper()

	if err := hub.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-serverErrors:
		if err != nil && !strings.Contains(err.Error(), "server is not running") {
			t.Fatalf("hub run failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("hub did not stop")
	}
}
