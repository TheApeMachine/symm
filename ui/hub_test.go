package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	. "github.com/smartystreets/goconvey/convey"
)

func TestWriteDropOldest(t *testing.T) {
	Convey("Write never blocks and drops the oldest frame", t, func() {
		hub := &Hub{frameCh: make(chan []byte, 2)}
		hub.connected.Store(true)
		hub.conn.Store(&websocket.Conn{})

		n, err := hub.Write([]byte("a"))
		So(err, ShouldBeNil)
		So(n, ShouldEqual, 1)

		_, err = hub.Write([]byte("bb"))
		So(err, ShouldBeNil)
		_, err = hub.Write([]byte("ccc"))
		So(err, ShouldBeNil)

		So(string(<-hub.frameCh), ShouldEqual, "bb")
		So(string(<-hub.frameCh), ShouldEqual, "ccc")
		So(len(hub.frameCh), ShouldEqual, 0)

		done := make(chan struct{})

		go func() {
			defer close(done)

			for i := 0; i < 10000; i++ {
				hub.Write([]byte("x"))
			}
		}()

		finished := false

		select {
		case <-done:
			finished = true
		case <-time.After(2 * time.Second):
		}

		So(finished, ShouldBeTrue)
	})
}

func TestWriteSkipsWhenIdle(t *testing.T) {
	Convey("Write drops empty and detached frames", t, func() {
		hub := &Hub{frameCh: make(chan []byte, 1)}

		n, err := hub.Write(nil)
		So(err, ShouldBeNil)
		So(n, ShouldEqual, 0)

		n, err = hub.Write([]byte("a"))
		So(err, ShouldBeNil)
		So(n, ShouldEqual, 1)
		So(len(hub.frameCh), ShouldEqual, 0)
	})
}

func TestDrainFrames(t *testing.T) {
	Convey("drainFrames clears leftovers without blocking", t, func() {
		hub := &Hub{frameCh: make(chan []byte, 2)}
		hub.frameCh <- []byte("stale")
		hub.frameCh <- []byte("older")

		hub.drainFrames()

		So(len(hub.frameCh), ShouldEqual, 0)
	})
}

func TestQueueFramesFromConfig(t *testing.T) {
	Convey("newHub sizes frameCh from the queue argument", t, func() {
		hub := newHub(context.Background(), 64)

		So(cap(hub.frameCh), ShouldEqual, 64)
	})
}
