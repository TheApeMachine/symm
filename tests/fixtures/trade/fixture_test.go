package trade

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the trade fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one trade snapshot frame with history", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				So(frame["channel"], ShouldEqual, "trade")
				So(frame["type"], ShouldEqual, "snapshot")
				So(len(frame["data"].([]any)), ShouldBeGreaterThan, 1)
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate an ordered trade sequence", func() {
				tradeID := uint64(0)
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					row := frame["data"].([]any)[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(uint64(row["trade_id"].(float64)), ShouldBeGreaterThan, tradeID)
					tradeID = uint64(row["trade_id"].(float64))
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestFixtureFrames(t *testing.T) {
	Convey("Given a trade update fixture", t, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When frames are requested", func() {
			count := 0

			for frame := range fixture.Frames() {
				So(frame.Channel, ShouldEqual, "trade")
				So(frame.Type, ShouldEqual, "update")
				count++
			}

			Convey("Then every generated frame should be typed", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})
}
