package http

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	gorillaws "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/ui"
	"testing"
	"time"
)

func TestHTTPServerPublish(t *testing.T) {
	Convey("The configured UI capability delivers and replays real binding frames", t, func() {
		server := NewHTTPServer(context.Background())
		client := HTTPServer_ServerToClient(server)
		defer client.Release()
		So(client.Write(context.Background(), func(args HTTPServer_write_Params) error { return args.SetAddress("127.0.0.1:0") }), ShouldBeNil)
		So(client.WaitStreaming(), ShouldBeNil)
		send := func(value string) {
			message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)
			defer message.Release()
			bindings, err := ui.NewRootBindings(segment)
			So(err, ShouldBeNil)
			values, err := bindings.NewValues(1)
			So(err, ShouldBeNil)
			So(values.At(0).SetGraph("ui_dashboard"), ShouldBeNil)
			So(values.At(0).SetComponent("price"), ShouldBeNil)
			So(values.At(0).SetProp("value"), ShouldBeNil)
			So(values.At(0).SetValue(value), ShouldBeNil)
			payload, err := message.Marshal()
			So(err, ShouldBeNil)
			future, release := client.Publish(context.Background(), func(args ui.Receiver_publish_Params) error { return args.SetData(payload) })
			defer release()
			_, err = future.Struct()
			So(err, ShouldBeNil)
		}
		send("101.25")
		connection, response, err := gorillaws.DefaultDialer.Dial("ws://"+server.address+"/ws", nil)
		So(err, ShouldBeNil)
		if response != nil {
			So(response.Body.Close(), ShouldBeNil)
		}
		defer func() { So(connection.Close(), ShouldBeNil) }()
		receive := func(expected string) {
			So(connection.SetReadDeadline(time.Now().Add(5*time.Second)), ShouldBeNil)
			kind, payload, err := connection.ReadMessage()
			So(err, ShouldBeNil)
			So(kind, ShouldEqual, gorillaws.BinaryMessage)
			message, err := capnp.Unmarshal(payload)
			So(err, ShouldBeNil)
			defer message.Release()
			bindings, err := ui.ReadRootBindings(message)
			So(err, ShouldBeNil)
			values, err := bindings.Values()
			So(err, ShouldBeNil)
			So(values.Len(), ShouldEqual, 1)
			value, err := values.At(0).Value()
			So(err, ShouldBeNil)
			So(value, ShouldEqual, expected)
		}
		receive("101.25")
		send("103.75")
		receive("103.75")
	})
}
