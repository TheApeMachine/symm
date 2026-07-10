package book

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the book fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one full book snapshot frame", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				row := frame["data"].([]any)[0].(map[string]any)
				bids := row["bids"].([]any)
				asks := row["asks"].([]any)
				bid := bids[0].(map[string]any)
				ask := asks[0].(map[string]any)

				So(frame["channel"], ShouldEqual, "book")
				So(frame["type"], ShouldEqual, "snapshot")
				So(bids, ShouldHaveLength, 10)
				So(asks, ShouldHaveLength, 10)
				So(bid["price"].(float64), ShouldBeLessThan, ask["price"].(float64))
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate an ordered partial book sequence", func() {
				checksum := uint64(0)
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					row := frame["data"].([]any)[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(row["bids"].([]any), ShouldHaveLength, 1)
					So(uint64(row["checksum"].(float64)), ShouldBeGreaterThan, checksum)
					checksum = uint64(row["checksum"].(float64))
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestFixtureFrames(t *testing.T) {
	Convey("Given a book update fixture", t, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When frames are requested", func() {
			count := 0

			for frame := range fixture.Frames() {
				So(frame.Channel, ShouldEqual, "book")
				So(frame.Type, ShouldEqual, "update")
				count++
			}

			Convey("Then every generated frame should be typed", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})

	Convey("Given a book snapshot fixture", t, func() {
		fixture := NewFixture(SNAPSHOT, 1)

		Convey("When frames are requested", func() {
			for frame := range fixture.Frames() {
				So(frame.Channel, ShouldEqual, "book")
				So(frame.Type, ShouldEqual, "snapshot")
			}
		})
	})
}
