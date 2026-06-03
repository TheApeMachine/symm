package public

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestNewWebSocketSingleton(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a pool and stream set", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		streams := focus.NewSet()

		first := NewWebSocket(ctx, pool, streams)
		second := NewWebSocket(ctx, pool, streams)

		Convey("It should return the process-wide socket", func() {
			So(first, ShouldEqual, second)
			So(first.latencies, ShouldNotBeNil)
		})
	})
}
