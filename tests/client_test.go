package tests

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewConn(t *testing.T) {
	Convey("Given a new Conn instantiation", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()

		So(conn, ShouldNotBeNil)
		So(conn.Client(), ShouldNotBeNil)
	})
}

func TestConnConfigure(t *testing.T) {
	Convey("Given a Conn", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()

		symbols := []*Symbol{
			NewSymbol("BTC/USD", 50000, 1),
		}

		conn.Configure(symbols)

		So(conn.transport.getSymbols(), ShouldResemble, symbols)
	})
}

func TestConnPublish(t *testing.T) {
	Convey("Given a Conn with an OnReceived listener", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()

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

		Convey("When a subscribe request is sent via OnSent", func() {
			ackReceived := false

			conn.Client().OnReceived.Recurring(
				func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
					ackReceived = true
				},
			)

			conn.Client().OnSent.Call(
				sdkkraken.NewWebSocketMessage(
					[]byte(`{"method":"subscribe","params":{"channel":"ticker"}}`),
				),
			)

			So(ackReceived, ShouldBeTrue)
		})
	})
}

func TestConnOrderACK(t *testing.T) {
	Convey("Given a Conn", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()

		Convey("When an add_order request is sent via OnSent", func() {
			ackReceived := false

			conn.Client().OnReceived.Recurring(
				func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
					ackReceived = true
				},
			)

			conn.Client().OnSent.Call(
				sdkkraken.NewWebSocketMessage(
					[]byte(`{"method":"add_order","cl_ord_id":"TEST_ORDER"}`),
				),
			)

			So(ackReceived, ShouldBeTrue)
		})
	})
}

func BenchmarkConnPublish(b *testing.B) {
	conn := NewConn(context.Background())
	defer conn.Close()

	payload := []byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD","price":50000}]}`)

	for b.Loop() {
		conn.Publish("ticker", payload)
	}
}
