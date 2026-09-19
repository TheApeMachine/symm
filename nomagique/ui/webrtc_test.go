package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestWebRTCServer(t *testing.T) {
	Convey("Given a WebRTCServer closure node", t, func() {
		server := NewWebRTCServer(types.Const(":18766"), types.Const("/webrtc"))
		So(server, ShouldNotBeNil)

		Convey("When input payload arrives", func() {
			out := server("test-frame")
			So(out, ShouldEqual, "test-frame")
		})
	})
}
