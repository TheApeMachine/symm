package websocket

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamsAdvance(t *testing.T) {
	Convey("Given frames observed on a transport connection", t, func() {
		streams := NewStreams("wss://futures.example")
		first := streams.Next("ticker")
		second := streams.Next("ticker")

		Convey("Advancing should start the next epoch at its first sequence", func() {
			streams.Advance()
			afterReconnect := streams.Next("ticker")

			So(first.Epoch, ShouldEqual, 1)
			So(first.Sequence, ShouldEqual, 1)
			So(second.Epoch, ShouldEqual, 1)
			So(second.Sequence, ShouldEqual, 2)
			So(afterReconnect.Epoch, ShouldEqual, 2)
			So(afterReconnect.Sequence, ShouldEqual, 1)
		})
	})
}

func BenchmarkStreamsAdvance(b *testing.B) {
	streams := NewStreams("wss://futures.example")
	streams.Next("ticker")
	streams.Next("trade")
	b.ReportAllocs()

	for iteration := 0; iteration < b.N; iteration++ {
		streams.Advance()
	}
}
