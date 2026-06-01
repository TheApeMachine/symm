package public

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewWebSocket(t *testing.T) {
	convey.Convey("Given a parent context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		convey.Convey("It should derive a websocket client", func() {
			socket, err := NewWebSocket(ctx)

			convey.So(err, convey.ShouldBeNil)
			convey.So(socket, convey.ShouldNotBeNil)
			convey.So(socket.ctx, convey.ShouldNotBeNil)
		})
	})
}

func TestStream(t *testing.T) {
	convey.Convey("Given a websocket without a connection", t, func() {
		ctx := context.Background()
		socket, err := NewWebSocket(ctx)

		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should reject Stream for unknown channels", func() {
			_, err := socket.Stream("ticker")
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestEmitDataRows(t *testing.T) {
	convey.Convey("Given one book envelope with two rows", t, func() {
		message := &SocketMessage{
			Channel: "book",
			Type:    "snapshot",
			Data: []byte(`[
				{"symbol":"BTC/EUR","bids":[{"price":100,"qty":1}],"asks":[{"price":101,"qty":2}]},
				{"symbol":"ETH/EUR","bids":[{"price":99,"qty":3}],"asks":[{"price":101,"qty":4}]}
			]`),
		}
		out := make(chan *SocketMessage, 4)

		err := emitDataRows(context.Background(), message, out)

		close(out)

		convey.Convey("It should preserve the envelope type on each row", func() {
			convey.So(err, convey.ShouldBeNil)

			first := <-out
			second := <-out

			convey.So(first.Type, convey.ShouldEqual, "snapshot")
			convey.So(second.Type, convey.ShouldEqual, "snapshot")
			convey.So(first.Channel, convey.ShouldEqual, "book")
			convey.So(len(first.Data), convey.ShouldBeGreaterThan, 0)
		})
	})
}
