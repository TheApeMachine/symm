package signal

import (
	"context"
	"time"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestBookResyncCooldown(t *testing.T) {
	Convey("Given a book resync requester", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		outbound := pool.CreateBroadcastGroup("kraken:public", time.Second)
		wire := outbound.Subscribe("test:resync:wire", 16)
		resync := NewBookResync(pool, 10)

		resync.Request("BTC/EUR")
		resync.Request("BTC/EUR") // inside the cooldown window: deduped
		resync.Request("ETH/EUR")
		resync.Request("") // ignored

		Convey("It should hold one cooldown window per symbol", func() {
			So(len(*resync.lastReq.Load()), ShouldEqual, 2)
		})

		Convey("It should put exactly one unsubscribe+subscribe pair per symbol on the wire", func() {
			frames := drainWireFrames(wire, 8)

			So(len(frames), ShouldEqual, 4) // 2 symbols × (unsubscribe + subscribe)
			So(frames[0]["method"], ShouldEqual, "unsubscribe")
			So(frames[1]["method"], ShouldEqual, "subscribe")

			params, ok := frames[1]["params"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(params["channel"], ShouldEqual, "book")
			So(params["depth"], ShouldEqual, 10)
			So(params["snapshot"], ShouldEqual, true)
			So(params["symbol"], ShouldResemble, []string{"BTC/EUR"})
		})
	})
}

// drainWireFrames polls up to limit frames without blocking — the dedupe
// assertion needs "nothing else arrived", which a parked Wait cannot express.
func drainWireFrames(consumer *qpool.BroadcastConsumer, limit int) []map[string]any {
	frames := make([]map[string]any, 0, limit)

	for range limit {
		message := consumer.Poll()

		if message == nil {
			break
		}

		frame, ok := message.Value.(map[string]any)

		if !ok {
			continue
		}

		frames = append(frames, frame)
	}

	return frames
}
