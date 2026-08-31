package ui

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
TestHubStepNonBlocking proves a slow or dead browser cannot stall Hub.Step: the
per-client publication boundary is bounded and non-blocking, so after semantic
state is committed no external observer can backpressure computations.
*/
func TestHubStepNonBlocking(t *testing.T) {
	Convey("Given a hub with a client that never drains", t, func() {
		hub := &Hub{
			clients: &sync.Map{},
			fluid:   NewFluidRTC(t.Context(), "hub-test"),
		}

		blocked := &Client{
			queue:  make(chan []byte, 1),
			closed: make(chan struct{}),
		}
		hub.clients.Store("blocked", blocked)

		started := time.Now()

		envelope := &types.Envelope{Key: "TEST/USD"}

		hub.Step(envelope)

		elapsed := time.Since(started)

		Convey("Step returns promptly instead of waiting on the socket", func() {
			So(elapsed, ShouldBeLessThan, 500*time.Millisecond)
		})

		Convey("the pending queue holds exactly one replaceable snapshot", func() {
			So(len(blocked.queue), ShouldEqual, 1)
		})
	})
}

/*
TestClientEnqueueLatestWins proves stale pending snapshots are replaced rather
than queued, so a slow viewer receives the freshest frame when it catches up.
*/
func TestClientEnqueueLatestWins(t *testing.T) {
	Convey("Given a client with a bounded publication boundary", t, func() {
		client := &Client{
			queue:  make(chan []byte, 1),
			closed: make(chan struct{}),
		}

		client.enqueue([]byte("stale"))
		client.enqueue([]byte("fresh"))

		Convey("the stale pending snapshot is replaced by the fresh one", func() {
			So(len(client.queue), ShouldEqual, 1)

			payload := <-client.queue
			So(string(payload), ShouldEqual, "fresh")
		})
	})
}
