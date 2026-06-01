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
			socket, err := NewWebSocket(ctx, pool)

			convey.So(err, convey.ShouldBeNil)
			convey.So(socket, convey.ShouldNotBeNil)
			convey.So(socket.ctx, convey.ShouldNotBeNil)
		})
	})
}
