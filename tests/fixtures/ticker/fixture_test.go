package ticker

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(testingTB *testing.T) {
	Convey("Given the ticker fixture package", testingTB, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one documented ticker snapshot frame", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				data := frame["data"].([]any)
				row := data[0].(map[string]any)

				So(frame["channel"], ShouldEqual, "ticker")
				So(frame["type"], ShouldEqual, "snapshot")
				So(data, ShouldHaveLength, 1)
				So(row["symbol"], ShouldEqual, "ALGO/USD")
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate a realistic ordered ticker sequence", func() {
				last := 0.0
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					data := frame["data"].([]any)
					row := data[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(data, ShouldHaveLength, 1)
					So(row["last"].(float64), ShouldBeGreaterThan, last)
					last = row["last"].(float64)
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestFixtureFrames(testingTB *testing.T) {
	Convey("Given a ticker update fixture", testingTB, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When frames are requested", func() {
			count := 0

			for frame := range fixture.Frames() {
				So(frame.Channel, ShouldEqual, "ticker")
				So(frame.Type, ShouldEqual, "update")
				count++
			}

			Convey("Then every generated frame should be typed", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})

	Convey("Given a ticker snapshot fixture", testingTB, func() {
		fixture := NewFixture(SNAPSHOT, 1)

		Convey("When frames are requested", func() {
			for frame := range fixture.Frames() {
				So(frame.Channel, ShouldEqual, "ticker")
				So(frame.Type, ShouldEqual, "snapshot")
			}
		})
	})
}
