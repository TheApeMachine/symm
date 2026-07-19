package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFrameKey(t *testing.T) {
	Convey("Given typed UI payloads", t, func() {
		Convey("It keeps graphs and lifecycle on distinct coalesce keys", func() {
			So(frameKey([]byte(`{"graphs":[]}`)), ShouldEqual, "graphs")
			So(frameKey([]byte(`{"lifecycle":{}}`)), ShouldEqual, "lifecycle")
			So(frameKey([]byte(`{"findings":[]}`)), ShouldEqual, "findings")
			So(frameKey([]byte(`{"holdings":[]}`)), ShouldEqual, "holdings")
		})
	})
}
