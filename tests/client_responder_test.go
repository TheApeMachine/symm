package tests

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/callback"
	sdkkraken "github.com/theapemachine/api-go/v2/pkg/kraken"
)

func TestFixtureResponderSubscribe(t *testing.T) {
	Convey("Given a connected fixture responder", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		So(conn.Connect(), ShouldBeNil)
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

		Convey("A subscribe request should receive a venue acknowledgement", func() {
			select {
			case ack := <-acked:
				So(string(ack), ShouldContainSubstring, `"method":"subscribe"`)
			case <-time.After(fixtureDeliveryTimeout):
				So("subscribe ack", ShouldEqual, "not received")
			}
		})
	})
}

func TestFixtureResponderAddOrder(t *testing.T) {
	Convey("Given a connected fixture responder", t, func() {
		conn := NewConn(context.Background())
		defer conn.Close()
		So(conn.Connect(), ShouldBeNil)
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

		Convey("An add-order request should receive a unique venue identity", func() {
			select {
			case ack := <-acked:
				So(string(ack), ShouldContainSubstring, "SIM-ORD-")
			case <-time.After(fixtureDeliveryTimeout):
				So("add_order ack", ShouldEqual, "not received")
			}
		})
	})
}
