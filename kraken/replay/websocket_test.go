package replay

import (
	"context"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestNewWebSocket(t *testing.T) {
	Convey("Given a replay capture file", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		capture := strings.NewReader(`{"channel":"ticker","type":"update","data":[]}` + "\n")
		socket, err := NewWebSocket(ctx, pool, capture)

		Convey("It should construct a replay websocket", func() {
			So(err, ShouldBeNil)
			So(socket, ShouldNotBeNil)
		})

		Convey("It should replay captured frames", func() {
			So(socket.Tick(), ShouldBeNil)
		})

		Convey("It should close cleanly", func() {
			So(socket.Close(), ShouldBeNil)
		})
	})
}
