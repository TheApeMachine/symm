package public

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestWriteOutboundFrameSubscribePace(t *testing.T) {
	Convey("Given a subscribe frame", t, func() {
		testconfig.Load(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		webSocket := &WebSocket{
			ctx:        ctx,
			broadcasts: map[string]*qpool.BroadcastGroup{},
		}

		frame := map[string]any{"method": "subscribe", "params": map[string]any{"channel": "ticker"}}

		Convey("It should require subscribe pace config when recording", func() {
			err := webSocket.writeOutboundFrame(frame, false)
			So(err, ShouldNotBeNil)
		})
	})
}

