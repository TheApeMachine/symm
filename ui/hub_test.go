package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

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
