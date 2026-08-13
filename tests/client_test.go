package tests

import (
	"context"
	"testing"

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
		So(conn.Client().URL, ShouldStartWith, "ws://")
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
	Convey("Given a Conn without a connected client", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()

		Convey("It should discard a frame that has no consumer", func() {
			conn.Publish("heartbeat", []byte(`{"channel":"heartbeat"}`))

			So(conn.accepted, ShouldBeNil)
		})
	})

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
