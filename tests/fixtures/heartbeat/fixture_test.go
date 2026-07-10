package heartbeat

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the heartbeat fixture package", t, func() {
		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate heartbeat frames", func() {
				count := 0

				for payload := range fixture.Generate() {
					So(strings.TrimSpace(string(payload)), ShouldEqual, `{"channel":"heartbeat"}`)
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestFixtureFrames(t *testing.T) {
	Convey("Given a heartbeat update fixture", t, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When frames are requested", func() {
			count := 0

			for frame := range fixture.Frames() {
				So(frame.Channel, ShouldEqual, "heartbeat")
				count++
			}

			Convey("Then every generated frame should be typed", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})
}
