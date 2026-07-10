package balances

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the balances fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one balance snapshot frame", func() {
				var frame map[string]any
				count := 0

				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					count++
				}

				So(count, ShouldEqual, 1)
				So(frame["channel"], ShouldEqual, "balances")
				So(frame["type"], ShouldEqual, "snapshot")
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate balance update frames", func() {
				sequence := float64(-1)
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					So(frame["channel"], ShouldEqual, "balances")
					So(frame["type"], ShouldEqual, "update")
					So(frame["sequence"].(float64), ShouldBeGreaterThan, sequence)
					sequence = frame["sequence"].(float64)
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestFixtureFrames(t *testing.T) {
	Convey("Given a balances update fixture", t, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When frames are requested", func() {
			count := 0

			for frame := range fixture.Frames() {
				So(frame.Channel, ShouldEqual, "balances")
				So(frame.Type, ShouldEqual, "update")
				count++
			}

			Convey("Then every generated frame should be typed", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})
}
