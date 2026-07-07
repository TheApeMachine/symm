package websocket

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamReceiveMethodResponse(testingTB *testing.T) {
	Convey("Given a stream observing an add_order method response", testingTB, func() {
		stream := NewStream(1)
		orders := stream.Observe("add_order")
		frame := []byte(`{
			"method": "add_order",
			"result": {"order_id": "O1"},
			"success": true,
			"req_id": 7
		}`)

		channel := stream.Receive(frame)

		Convey("Then the raw response should publish under the method name", func() {
			So(channel, ShouldEqual, "add_order")

			select {
			case observed := <-orders:
				So(string(observed), ShouldEqual, string(frame))
			case <-time.After(time.Second):
				testingTB.Fatal("add_order response was not published")
			}
		})
	})
}

func BenchmarkStreamReceive(benchmarkTB *testing.B) {
	stream := NewStream(64)
	ticker := stream.Observe("ticker")
	frame := []byte(`{
		"channel": "ticker",
		"type": "update",
		"data": [{"symbol": "BTC/USD", "last": 100.0}]
	}`)

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		stream.Receive(frame)
		<-ticker
	}
}
