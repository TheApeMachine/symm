package replay

import (
	"context"
	"testing"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
)

func TestServerDeliver(t *testing.T) {
	Convey("A websocket peer receives original payload bytes through subscribed channels", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		server := NewServer(ctx, map[string]string{"wss://venue": "level3"})
		client, response, err := websocket.DefaultDialer.Dial(server.URL("level3"), nil)
		So(err, ShouldBeNil)
		So(response.Body.Close(), ShouldBeNil)
		defer func() {
			cancel()
			So(client.Close(), ShouldBeNil)
			So(server.Close(), ShouldBeNil)
		}()
		_, frames := captureFixture(t, 1)
		frame := frames[0]
		So(server.Deliver(frame), ShouldBeNil)
		_, received, err := client.ReadMessage()
		So(err, ShouldBeNil)
		So(received, ShouldResemble, frame.Payload)

		Convey("The matching subscription owns a recorded book frame", func() {
			So(client.WriteMessage(websocket.TextMessage, []byte(`{"method":"subscribe","params":{"channel":"level3","symbol":["BTC/USD"]}}`)), ShouldBeNil)
			frame.Kind = "level3"
			frame.Payload = []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[],"asks":[]}]}`)
			So(server.Deliver(frame), ShouldBeNil)
			_, received, err := client.ReadMessage()
			So(err, ShouldBeNil)
			So(received, ShouldResemble, frame.Payload)
			So(server.Frames, ShouldEqual, 2)
		})
		Convey("Derived touch witnesses are counted without injecting them twice", func() {
			frame.Kind = "l3_touch"
			So(server.Deliver(frame), ShouldBeNil)
			So(server.Derived, ShouldEqual, 1)
			So(server.Frames, ShouldEqual, 1)
		})
		Convey("Unknown transport origins fail explicitly", func() {
			frame.Endpoint = "wss://unknown"
			So(server.Deliver(frame), ShouldNotBeNil)
		})
	})
}
