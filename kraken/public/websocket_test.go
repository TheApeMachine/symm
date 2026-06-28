package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

func TestNewWebSocket(t *testing.T) {
	Convey("Given a public websocket context", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		tree := dmt.NewTree("")

		Convey("When creating a WebSocket", func() {
			ws := newTestWebSocket(t.Context(), pool, tree)
			defer ws.Close()

			Convey("Then it should initialize the websocket state", func() {
				_, ticker := ws.broadcasts.Load("ticker")
				_, subscriber := ws.subscribers.Load("kraken:public")

				So(ws.ctx, ShouldNotBeNil)
				So(ws.cancel, ShouldNotBeNil)
				So(ws.pool, ShouldEqual, pool)
				So(ws.tree, ShouldEqual, tree)
				So(ws.instrument, ShouldNotBeNil)
				So(ticker, ShouldBeTrue)
				So(subscriber, ShouldBeTrue)
			})
		})
	})
}

func TestOnMessage(t *testing.T) {
	Convey("Given a public websocket", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))
		defer ws.Close()

		payload := datura.Map[any]{
			"method": "subscribe",
			"params": datura.Map[any]{
				"channel": "ticker",
			},
		}.Marshal()

		Convey("When a kraken public message arrives before connection", func() {
			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"kraken:public",
			).WithPayload(
				payload,
			))

			Convey("Then it should reject the write", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When a message is addressed elsewhere", func() {
			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"desk",
			).WithPayload(
				payload,
			))

			Convey("Then it should ignore the destination as an error", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When a kraken public message arrives after connection", func() {
			received := make(chan []byte, 1)
			server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
				var wire []byte
				if err := conn.ReadJSON(&wire); err != nil {
					t.Errorf("read json failed: %v", err)
					return
				}
				received <- wire
			})
			defer server.Close()

			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			ws.connectMaxDelay = 2
			So(ws.Connect(endpoint, 1), ShouldBeNil)

			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"kraken:public",
			).WithPayload(
				payload,
			))

			Convey("Then it should write the payload to the websocket", func() {
				So(err, ShouldBeNil)

				select {
				case wire := <-received:
					So(wire, ShouldResemble, payload)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for websocket payload")
				}
			})
		})
	})
}

func TestRun(t *testing.T) {
	Convey("Given a public websocket and a Kraken ticker frame", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		tree := dmt.NewTree("")
		ws := newTestWebSocket(ctx, pool, tree)
		ws.connectMaxDelay = 2
		defer ws.Close()

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(
				`{"channel":"ticker","type":"update","data":[{"symbol":"DOGE/USD","last":0.2}]}`,
			)); err != nil {
				t.Errorf("write ticker frame failed: %v", err)
				return
			}
			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Run reads the frame", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)

			artifact := waitForArtifact(t, tree, []byte("ticker/update/"))

			Convey("Then it should persist it by role, scope, and timestamp", func() {
				So(artifact, ShouldNotBeNil)
				So(datura.Peek[string](artifact, "role"), ShouldEqual, "ticker")
				So(datura.Peek[string](artifact, "scope"), ShouldEqual, "update")
				So(datura.Peek[string](artifact, "channel"), ShouldEqual, "ticker")
			})
		})
	})
}

func TestError(t *testing.T) {
	Convey("Given a public websocket with an error", t, func() {
		ws := newTestWebSocket(t.Context(), qpool.NewQ[any](t.Context(), 1, 1, nil), dmt.NewTree(""))
		defer ws.Close()
		ws.err = errnie.Err(errnie.Unknown, "test", nil)

		Convey("When Error is called", func() {
			err := ws.Error()

			Convey("Then it should return the stored error", func() {
				So(err, ShouldEqual, ws.err)
			})
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a connected public websocket", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			<-request.Context().Done()
		})
		defer server.Close()

		endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
		ws.connectMaxDelay = 2
		So(ws.Connect(endpoint, 1), ShouldBeNil)

		Convey("When Close is called", func() {
			err := ws.Close()

			Convey("Then it should close and clear the connection", func() {
				So(err, ShouldBeNil)
				So(ws.conn, ShouldBeNil)
				So(ws.isConnected.Load(), ShouldBeFalse)
			})
		})
	})
}

func TestConnect(t *testing.T) {
	Convey("Given a public websocket endpoint", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))
		defer ws.Close()

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Connect dials a websocket server", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			ws.connectMaxDelay = 2
			err := ws.Connect(endpoint, 1)

			Convey("Then it should mark the websocket connected", func() {
				So(err, ShouldBeNil)
				So(ws.conn, ShouldNotBeNil)
				So(ws.isConnected.Load(), ShouldBeTrue)
			})
		})

		Convey("When the attempt exceeds the max delay", func() {
			ws.connectMaxDelay = 0
			err := ws.Connect(WebSocketURL, 1)

			Convey("Then it should stop before dialing", func() {
				So(err, ShouldNotBeNil)
				So(ws.isConnected.Load(), ShouldBeFalse)
			})
		})
	})
}

func TestDisconnect(t *testing.T) {
	Convey("Given a connected public websocket", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))
		defer ws.Close()

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			<-request.Context().Done()
		})
		defer server.Close()

		endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
		ws.connectMaxDelay = 2
		So(ws.Connect(endpoint, 1), ShouldBeNil)

		Convey("When disconnect is called", func() {
			ws.disconnect()

			Convey("Then it should clear connection state", func() {
				So(ws.conn, ShouldBeNil)
				So(ws.isConnected.Load(), ShouldBeFalse)
			})
		})
	})
}

func newTestWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *WebSocket {
	return NewWebSocket(
		ctx,
		pool,
		tree,
		nil,
		[]string{"ticker"},
		[]string{"kraken:public"},
	)
}

func newWebSocketServer(
	t *testing.T,
	handle func(*websocket.Conn, *http.Request),
) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			return true
		},
	}

	return httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			conn, err := upgrader.Upgrade(response, request, nil)
			if err != nil {
				t.Errorf("upgrade failed: %v", err)
				return
			}
			defer conn.Close()

			handle(conn, request)
		},
	))
}

func waitForArtifact(
	t *testing.T,
	tree *dmt.Tree,
	prefix []byte,
) *datura.Artifact {
	t.Helper()

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		for artifact := range tree.Seek(prefix) {
			return artifact
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for artifact under %q", string(prefix))
	return nil
}
