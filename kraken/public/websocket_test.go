package public

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestNewWebSocket(t *testing.T) {
	convey.Convey("Given a parent context and pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)

		convey.Convey("It should derive a websocket client", func() {
			socket := NewWebSocket(ctx, pool)

			convey.So(socket, convey.ShouldNotBeNil)
			convey.So(socket.ctx, convey.ShouldNotBeNil)
			convey.So(socket.cancel, convey.ShouldNotBeNil)
			convey.So(socket.pool, convey.ShouldNotBeNil)
			convey.So(socket.broadcasts, convey.ShouldNotBeNil)
			convey.So(socket.subscribers, convey.ShouldNotBeNil)
			convey.So(socket.recorder, convey.ShouldNotBeNil)
		})
	})
}
