package public

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestWriteOutboundBeforeConnect(t *testing.T) {
	Convey("Given a public websocket that is not connected", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		socket := NewWebSocket(ctx, pool)

		payload := []byte(`{"method":"subscribe","params":{"channel":"instrument"}}`)

		Convey("When writeOutbound is called", func() {
			socket.writeOutbound(payload)

			Convey("It should no-op without panicking", func() {
				So(socket.isConnected.Load(), ShouldBeFalse)
			})
		})
	})
}
