package ui

import (
	"errors"
	"net"
	"syscall"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHeartbeatFrame(t *testing.T) {
	Convey("Given heartbeatFrame", t, func() {
		hub := &Hub{}

		frame := hub.heartbeatFrame(42)

		Convey("It should emit the wire heartbeat shape", func() {
			So(frame["event"], ShouldEqual, "heartbeat")
			So(frame["seq"], ShouldEqual, 42)
			So(frame["ts"], ShouldNotBeBlank)
		})
	})
}

func TestClientDisconnected(t *testing.T) {
	Convey("Given clientDisconnected", t, func() {
		Convey("It should treat broken pipe and reset as disconnect", func() {
			So(clientDisconnected(errors.New("write: broken pipe")), ShouldBeTrue)
			So(clientDisconnected(errors.New("read: connection reset by peer")), ShouldBeTrue)
			So(
				clientDisconnected(&net.OpError{Err: syscall.EPIPE}),
				ShouldBeTrue,
			)
		})

		Convey("It should not treat other errors as disconnect", func() {
			So(clientDisconnected(errors.New("timeout")), ShouldBeFalse)
			So(clientDisconnected(nil), ShouldBeFalse)
		})
	})
}
