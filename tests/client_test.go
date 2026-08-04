package tests

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests/types"
)

func TestNewConn(t *testing.T) {
	Convey("Given a new Conn instantiation", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()

		So(conn, ShouldNotBeNil)
		So(conn.Client(), ShouldNotBeNil)
		So(conn.Connect(), ShouldBeNil)
	})
}

func TestConnConfigure(t *testing.T) {
	Convey("Given a Conn", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()

		symbols := []*types.Symbol{
			types.NewSymbol("BTC/USD", 50000, 1),
		}

		conn.Configure(symbols)

		So(conn.transport.getSymbols(), ShouldResemble, symbols)
	})
}

func TestConnPublish(t *testing.T) {
	Convey("Given a Conn with an OnReceived listener", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		So(conn.Connect(), ShouldBeNil)

		var received []byte

		conn.Client().OnReceived.Recurring(
			func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
				received = event.Data.Bytes()
			},
		)

		Convey("When Publish is called with a payload", func() {
			payload := []byte(`{"channel":"heartbeat"}`)
			conn.Publish("heartbeat", payload)

			So(string(received), ShouldEqual, string(payload))
		})
	})
}

func TestConnSubscriptionACK(t *testing.T) {
	Convey("Given a Conn", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		So(conn.Connect(), ShouldBeNil)

		Convey("When a subscribe request is written to the connection", func() {
			acked := make(chan []byte, 1)

			conn.Client().OnReceived.Recurring(
				func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
					select {
					case acked <- event.Data.Bytes():
					default:
					}
				},
			)

			So(conn.Client().WriteMessage(
				websocket.TextMessage,
				[]byte(`{"method":"subscribe","params":{"channel":"ticker"}}`),
			), ShouldBeNil)

			select {
			case ack := <-acked:
				So(string(ack), ShouldContainSubstring, `"method":"subscribe"`)
			case <-time.After(5 * time.Second):
				So("subscribe ack", ShouldEqual, "not received")
			}
		})
	})
}

func TestConnOrderACK(t *testing.T) {
	Convey("Given a Conn", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		So(conn.Connect(), ShouldBeNil)

		Convey("When an add_order request is written to the connection", func() {
			acked := make(chan []byte, 1)

			conn.Client().OnReceived.Recurring(
				func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
					select {
					case acked <- event.Data.Bytes():
					default:
					}
				},
			)

			So(conn.Client().WriteMessage(
				websocket.TextMessage,
				[]byte(`{"method":"add_order","cl_ord_id":"TEST_ORDER"}`),
			), ShouldBeNil)

			select {
			case ack := <-acked:
				So(string(ack), ShouldContainSubstring, "SIM-ORD-")
			case <-time.After(5 * time.Second):
				So("add_order ack", ShouldEqual, "not received")
			}
		})
	})
}

func BenchmarkConnPublish(b *testing.B) {
	conn := NewConn(context.Background())
	defer conn.Close()

	if err := conn.Connect(); err != nil {
		b.Fatal(err)
	}

	payload := []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","price":50000}]}`)

	for b.Loop() {
		conn.Publish("ticker", payload)
	}
}
