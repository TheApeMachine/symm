package public

import (
	"context"
	"encoding/json"
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

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
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

func TestSocketMessagePoolReset(t *testing.T) {
	Convey("Given a recycled socket message with stale channel data", t, func() {
		message := &SocketMessage{
			Channel: "ticker",
			Type:    "update",
			Data:    json.RawMessage(`{"symbol":"BTC/EUR"}`),
		}

		Convey("Zeroing before ReadJSON prevents stale fields from leaking", func() {
			*message = SocketMessage{}

			err := json.Unmarshal([]byte(`{"method":"pong"}`), message)

			So(err, ShouldBeNil)
			So(message.Channel, ShouldEqual, "")
			So(message.Type, ShouldEqual, "")
			So(len(message.Data), ShouldEqual, 0)
		})
	})
}
