package ui

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	. "github.com/smartystreets/goconvey/convey"
)

func TestWrite(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the hub is connected with a two-slot queue", func() {
			hub := &Hub{frameCh: make(chan []byte, 2)}
			hub.connected.Store(true)
			hub.conn.Store(&websocket.Conn{})

			Convey("When three frames are written", func() {
				n, err := hub.Write([]byte("a"))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 1)

				_, err = hub.Write([]byte("bb"))
				So(err, ShouldBeNil)
				_, err = hub.Write([]byte("ccc"))
				So(err, ShouldBeNil)

				Convey("The oldest frame is dropped and the queue holds the last two", func() {
					So(string(<-hub.frameCh), ShouldEqual, "bb")
					So(string(<-hub.frameCh), ShouldEqual, "ccc")
					So(len(hub.frameCh), ShouldEqual, 0)
				})
			})

			Convey("When many frames are written concurrently", func() {
				done := make(chan struct{})

				go func() {
					defer close(done)

					for range 10000 {
						hub.Write([]byte("x"))
					}
				}()

				finished := false

				select {
				case <-done:
					finished = true
				case <-time.After(2 * time.Second):
				}

				Convey("Write never blocks the caller", func() {
					So(finished, ShouldBeTrue)
				})
			})
		})

		Convey("When the hub is detached", func() {
			hub := &Hub{frameCh: make(chan []byte, 1)}

			Convey("When empty or non-active writes are enqueued", func() {
				n, err := hub.Write(nil)
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 0)

				n, err = hub.Write([]byte("a"))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 1)

				Convey("Nothing is queued", func() {
					So(len(hub.frameCh), ShouldEqual, 0)
				})
			})
		})
	})
}

func TestDrainFrames(t *testing.T) {
	Convey("Setup", t, func() {
		hub := &Hub{frameCh: make(chan []byte, 2)}
		hub.frameCh <- []byte("stale")
		hub.frameCh <- []byte("older")

		Convey("When drainFrames is called", func() {
			hub.drainFrames()

			Convey("The channel is empty", func() {
				So(len(hub.frameCh), ShouldEqual, 0)
			})
		})
	})
}

func TestNewHub(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When newHub is called with queue depth 64", func() {
			hub := newHub(context.Background(), 64)

			Convey("frameCh capacity matches the argument", func() {
				So(cap(hub.frameCh), ShouldEqual, 64)
			})
		})
	})
}

func TestActive(t *testing.T) {
	Convey("Setup", t, func() {
		hub := &Hub{frameCh: make(chan []byte, 1)}

		Convey("When a socket is connected", func() {
			hub.connected.Store(true)
			hub.conn.Store(&websocket.Conn{})

			Convey("Active is true", func() {
				So(hub.Active(), ShouldBeTrue)
			})
		})

		Convey("When PrimeGrace opened the attach window", func() {
			restore := hub.PrimeGrace()

			Convey("Active is true within grace", func() {
				So(hub.Active(), ShouldBeTrue)
				restore()
			})
		})

		Convey("When neither connected nor graced", func() {
			Convey("Active is false", func() {
				So(hub.Active(), ShouldBeFalse)
			})
		})
	})
}

func TestNextFrame(t *testing.T) {
	Convey("Setup", t, func() {
		hub := &Hub{frameCh: make(chan []byte, 1)}

		Convey("When a frame is queued", func() {
			hub.frameCh <- []byte("tick")

			buf, ok := hub.NextFrame()

			Convey("NextFrame returns a copy", func() {
				So(ok, ShouldBeTrue)
				So(string(buf), ShouldEqual, "tick")
			})
		})

		Convey("When the queue is empty", func() {
			_, ok := hub.NextFrame()

			Convey("NextFrame reports none", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}
