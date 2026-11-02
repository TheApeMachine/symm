package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
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

func TestQueueFramesDefault(t *testing.T) {
	Convey("NewHub sizes frameCh from config and falls back to 256", t, func() {
		prev := system.Cfg.UI.WebSocket.QueueFrames
		system.Cfg.UI.WebSocket.QueueFrames = 0

		hub := NewHub(context.Background())

		system.Cfg.UI.WebSocket.QueueFrames = prev

		So(cap(hub.frameCh), ShouldEqual, 256)
	})
}

func TestStreamMaps(t *testing.T) {
	Convey("stream maps survive concurrent start and cancel", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		hub := &Hub{
			ctx:       ctx,
			streams:   make(map[string]context.CancelFunc),
			streamGen: make(map[string]uint64),
		}

		var wg sync.WaitGroup

		for i := 0; i < 32; i++ {
			wg.Add(2)

			go func() {
				defer wg.Done()
				hub.startStream("s3", func(ctx context.Context) {
					<-ctx.Done()
				})
			}()

			go func() {
				defer wg.Done()
				hub.cancelStream("s3")
			}()
		}

		wg.Wait()
		cancel()

		hub.streamMu.Lock()
		So(hub.streams, ShouldNotBeNil)
		hub.streamMu.Unlock()
	})
}
