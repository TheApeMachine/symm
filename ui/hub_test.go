package ui

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"
)

/*
TestClientGoneTreatsFlateDesyncAsDisconnect keeps compression/client-drop
failures out of the error overlay path.
*/
func TestClientGoneTreatsFlateDesyncAsDisconnect(t *testing.T) {
	Convey("Given a permessage-deflate desync from a dropped browser socket", t, func() {
		err := errors.New("websocket: internal error, unexpected bytes at end of flate stream")

		Convey("Then the hub treats it as client-gone", func() {
			hub := &Hub{
				status:   types.READY,
				Messages: make(chan []byte, 1),
			}

			So(hub.clientGone(err), ShouldBeTrue)
			So(hub.clientGone(errors.New("sonic: encode failed")), ShouldBeFalse)
		})
	})
}

/*
TestHubOpenWaitsForWarmupGate keeps the dashboard socket closed until the boot
callback reports ready.
*/
func TestHubOpenWaitsForWarmupGate(t *testing.T) {
	Convey("Given a hub gated on Warmup", t, func() {
		open := false
		hub := &Hub{
			status:   types.INITIALIZING,
			Messages: make(chan []byte, 1),
			ready:    func() bool { return open },
		}

		Convey("It rejects clients before the gate opens", func() {
			So(hub.open(), ShouldBeFalse)
		})

		Convey("It accepts clients once Warmup reports ready", func() {
			open = true
			So(hub.open(), ShouldBeTrue)
		})
	})
}

/*
TestHubInitializeStartsCoalescer ensures Serve cannot listen without the
flush worker — the failure mode that left the dashboard on a single seed frame.
*/
func TestHubInitializeStartsCoalescer(t *testing.T) {
	Convey("Given a fresh hub", t, func() {
		hub := &Hub{
			status:   types.INITIALIZING,
			Messages: make(chan []byte, 1),
			ctx:      t.Context(),
			cancel:   func() {},
		}

		So(hub.Initialize(), ShouldBeNil)
		So(hub.status, ShouldEqual, types.READY)
		So(hub.Initialize(), ShouldBeNil)
	})
}

/*
TestHubCloseCancelsBeforeShutdown proves Close cancels the hub context so the
"/ws" handler parked on ctx.Done unblocks. Ordering Shutdown first would wait on
that handler forever — the deadlock this teardown path must never reintroduce.
*/
func TestHubCloseCancelsBeforeShutdown(t *testing.T) {
	Convey("Given a hub with a live /ws client", t, func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		addr := listener.Addr().String()
		So(listener.Close(), ShouldBeNil)

		previousAddr := viper.Get("ui.addr")
		viper.Set("ui.addr", addr)
		t.Cleanup(func() { viper.Set("ui.addr", previousAddr) })

		hub, err := NewHub(
			context.Background(),
			nil,
			nil,
			nil,
			make(chan []byte, 1),
			func() bool { return true },
		)
		So(err, ShouldBeNil)
		So(hub.Initialize(), ShouldBeNil)

		serveErr := make(chan error, 1)

		go func() {
			serveErr <- hub.Serve()
		}()

		url := "ws://" + addr + "/ws"
		var conn *gorillawebsocket.Conn
		deadline := time.Now().Add(2 * time.Second)

		for time.Now().Before(deadline) {
			conn, _, err = gorillawebsocket.DefaultDialer.Dial(url, nil)

			if err == nil {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		So(err, ShouldBeNil)
		So(conn, ShouldNotBeNil)
		defer conn.Close()

		Convey("Close cancels the context and returns without blocking", func() {
			done := make(chan error, 1)

			go func() {
				done <- hub.Close()
			}()

			select {
			case closeErr := <-done:
				So(closeErr, ShouldBeNil)
			case <-time.After(2 * time.Second):
				t.Fatal("Close blocked waiting on the live /ws handler")
			}

			select {
			case <-hub.ctx.Done():
			default:
				t.Fatal("Close did not cancel the hub context")
			}
		})
	})
}

/*
TestHubIngestRetainsCurrentForReconnect proves the current/dirty split: an
ingested frame lands in both maps, and clearing the delivered dirty mark leaves
current intact so a reconnecting client is still seeded with the last good frame.
*/
func TestHubIngestRetainsCurrentForReconnect(t *testing.T) {
	Convey("Given a hub that ingested a keyed frame", t, func() {
		hub := &Hub{status: types.READY, Messages: make(chan []byte, 1)}
		frame := []byte(`{"manifold":{}}`)

		hub.ingest(frame)

		Convey("It is pending on dirty and retained on current", func() {
			_, current := hub.current.Load("manifold")
			_, dirty := hub.dirty.Load("manifold")
			So(current, ShouldBeTrue)
			So(dirty, ShouldBeTrue)
		})

		Convey("Delivering the frame clears dirty but keeps current", func() {
			hub.dirty.Delete("manifold")

			_, current := hub.current.Load("manifold")
			_, dirty := hub.dirty.Load("manifold")
			So(current, ShouldBeTrue)
			So(dirty, ShouldBeFalse)
		})

		Convey("Re-ingesting the key re-marks it dirty for the live client", func() {
			hub.dirty.Delete("manifold")
			hub.ingest([]byte(`{"manifold":{"v":1}}`))

			_, dirty := hub.dirty.Load("manifold")
			So(dirty, ShouldBeTrue)
		})
	})
}
