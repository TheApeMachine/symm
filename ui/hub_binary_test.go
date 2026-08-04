package ui

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
)

func TestIsManifoldBinary(t *testing.T) {
	Convey("Given an SMF1 lattice frame", t, func() {
		payload := []byte{'S', 'M', 'F', '1', 2, 0, 0}

		Convey("It is treated as binary fanout", func() {
			So(isManifoldBinary(payload), ShouldBeTrue)
		})
	})

	Convey("Given an SMF1 display frame", t, func() {
		payload := []byte{'S', 'M', 'F', '1', 5, 0, 0}

		Convey("It is treated as binary fanout", func() {
			So(isManifoldBinary(payload), ShouldBeTrue)
		})
	})

	Convey("Given a JSON UI frame", t, func() {
		So(isManifoldBinary([]byte(`{"balances":[]}`)), ShouldBeFalse)
	})
}

func TestHubManifold(t *testing.T) {
	Convey("Given a client connected to the manifold websocket", t, func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		messages := make(chan []byte, 1)
		manifold := make(chan []byte, 1)
		hub := NewHub(context.Background(), nil, nil, nil, messages, manifold)
		serveErr := make(chan error, 1)

		go func() {
			serveErr <- hub.app.Listener(listener)
		}()

		Reset(func() {
			hub.cancel()
			So(hub.Close(), ShouldBeNil)

			select {
			case err = <-serveErr:
				So(err, ShouldBeNil)
			case <-time.After(time.Second):
				t.Fail()
			}
		})

		conn, _, err := websocket.DefaultDialer.Dial(
			"ws://"+listener.Addr().String()+"/ws-manifold",
			nil,
		)
		So(err, ShouldBeNil)
		Reset(func() { So(conn.Close(), ShouldBeNil) })
		payload := []byte{'S', 'M', 'F', '1', 5, 0, 0}
		manifold <- payload
		So(conn.SetReadDeadline(time.Now().Add(time.Second)), ShouldBeNil)

		messageType, received, err := conn.ReadMessage()

		Convey("It should receive the payload as a binary websocket message", func() {
			So(err, ShouldBeNil)
			So(messageType, ShouldEqual, websocket.BinaryMessage)
			So(received, ShouldResemble, payload)
		})
	})
}
