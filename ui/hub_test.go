package ui

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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
