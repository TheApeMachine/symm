package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWsClientEnqueueAfterClosed(t *testing.T) {
	Convey("Given a client marked closed", t, func() {
		client := &wsClient{
			out:  make(chan any, 1),
			done: make(chan struct{}),
		}
		client.closed.Store(true)

		Convey("It should reject new frames", func() {
			So(client.enqueue("frame"), ShouldBeFalse)
			So(client.enqueuePriority("heartbeat"), ShouldBeFalse)
		})
	})
}

func BenchmarkWsClientEnqueue(b *testing.B) {
	client := &wsClient{
		out:  make(chan any, 16),
		done: make(chan struct{}),
	}

	for b.Loop() {
		client.enqueue("frame")
	}
}
