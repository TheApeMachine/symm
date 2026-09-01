package websocket

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theapemachine/symm/system"
	. "github.com/smartystreets/goconvey/convey"
)

/*
withPingInterval runs the case against a keepalive interval short enough to
observe, restoring whatever the process was configured with.
*/
func withPingInterval(interval time.Duration, body func()) {
	previous := system.Cfg.WebSocket.PingInterval
	system.Cfg.WebSocket.PingInterval = interval

	defer func() {
		system.Cfg.WebSocket.PingInterval = previous
	}()

	body()
}

func TestPingerStart(t *testing.T) {
	Convey("Given a session whose socket still accepts writes", t, func() {
		withPingInterval(time.Millisecond, func() {
			var sent atomic.Int64
			pinger := NewPinger("test", func() error {
				sent.Add(1)
				return nil
			})

			pinger.Start(context.Background())
			defer pinger.Stop()

			Convey("The keepalive keeps running", func() {
				time.Sleep(30 * time.Millisecond)

				So(sent.Load(), ShouldBeGreaterThan, 1)
			})
		})
	})

	Convey("Given a socket whose write side is gone", t, func() {
		withPingInterval(time.Millisecond, func() {
			var sent atomic.Int64
			var failures atomic.Int64
			pinger := NewPinger("test", func() error {
				sent.Add(1)
				return errors.New("broken pipe")
			})
			pinger.OnFailed(func(error) {
				failures.Add(1)
			})

			pinger.Start(context.Background())
			defer pinger.Stop()

			time.Sleep(30 * time.Millisecond)

			Convey("The failure is reported so the session can reconnect", func() {
				So(failures.Load(), ShouldEqual, 1)
			})

			Convey("And the loop stands down instead of writing forever", func() {
				So(sent.Load(), ShouldEqual, 1)
			})
		})
	})

	Convey("Given a cancelled session", t, func() {
		withPingInterval(time.Millisecond, func() {
			var sent atomic.Int64
			pinger := NewPinger("test", func() error {
				sent.Add(1)
				return nil
			})

			ctx, cancel := context.WithCancel(context.Background())
			pinger.Start(ctx)
			cancel()
			time.Sleep(20 * time.Millisecond)
			observed := sent.Load()
			time.Sleep(20 * time.Millisecond)

			Convey("The keepalive stops", func() {
				So(sent.Load(), ShouldEqual, observed)
			})
		})
	})
}
