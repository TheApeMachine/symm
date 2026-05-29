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

func TestWebSocketGenerate(t *testing.T) {
	convey.Convey("Given a websocket without a connection", t, func() {
		ctx := context.Background()
		socket, err := NewWebSocket(ctx)

		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should reject generate for unknown channels", func() {
			_, err := socket.Generate("ticker")

			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
