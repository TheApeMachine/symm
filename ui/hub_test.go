package ui

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHubShouldDrop(t *testing.T) {
	Convey("Given hub focus filtering", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		t.Cleanup(func() { pool.Close() })

		hub, err := NewHub(ctx, pool)
		So(err, ShouldBeNil)
		t.Cleanup(func() { hub.cancel() })

		Convey("It should keep global audit and prediction frames even when they carry symbols", func() {
			So(hub.shouldDrop(map[string]any{
				"event":  "audit",
				"symbol": "ETH/EUR",
			}), ShouldBeFalse)
			So(hub.shouldDrop(map[string]any{
				"event":  "prediction",
				"symbol": "ETH/EUR",
			}), ShouldBeFalse)
			So(hub.shouldDrop(map[string]any{
				"event":  "prediction_settled",
				"symbol": "ETH/EUR",
			}), ShouldBeFalse)
		})

		Convey("It should still drop unfocused per-symbol chart frames", func() {
			So(hub.shouldDrop(map[string]any{
				"event":  "mark",
				"symbol": "ETH/EUR",
			}), ShouldBeTrue)
		})
	})
}

func TestWsClientEnqueuePriority(t *testing.T) {
	Convey("Given a full client outbox", t, func() {
		client := &wsClient{
			out:  make(chan any, 1),
			done: make(chan struct{}),
		}
		client.out <- "heavy"

		ok := client.enqueuePriority("heartbeat")

		Convey("It should evict old telemetry for the priority heartbeat", func() {
			So(ok, ShouldBeTrue)
			So(<-client.out, ShouldEqual, "heartbeat")
		})
	})
}
