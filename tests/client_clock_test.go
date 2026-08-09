package tests

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConnConfigureTime(t *testing.T) {
	Convey("Given a deterministic fixture clock", t, func() {
		conn := NewConn(t.Context())
		defer conn.Close()
		start := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
		So(conn.ConfigureTime(start), ShouldBeNil)

		Convey("Websocket and REST responses should share the same anchor", func() {
			So(conn.currentTime(), ShouldEqual, start)
			So(conn.transport.clock, ShouldEqual, start)
			So(conn.ConfigureTime(time.Time{}), ShouldNotBeNil)
		})
	})
}
