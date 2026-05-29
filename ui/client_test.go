package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWsClientEnqueue(t *testing.T) {
	Convey("Given an open client outbox", t, func() {
		client := &wsClient{
			out:  make(chan any, 1),
			done: make(chan struct{}),
		}

		ok := client.enqueue("frame")

		Convey("It should enqueue the frame", func() {
			So(ok, ShouldBeTrue)
			So(<-client.out, ShouldEqual, "frame")
		})
	})
}
