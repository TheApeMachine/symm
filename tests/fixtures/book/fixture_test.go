package book

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewDecoderFixture(t *testing.T) {
	Convey("Given the book fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewDecoderFixture(SNAPSHOT, 1)

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
			fixture := NewDecoderFixture(UPDATE, 3)

			Convey("Then it should generate an ordered partial book sequence", func() {
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					row := frame["data"].([]any)[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(row["bids"].([]any), ShouldHaveLength, 1)
					So(uint64(row["checksum"].(float64)), ShouldBeGreaterThan, 0)
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})

		Convey("When checksum depth exceeds Kraken's ten-level CRC window", func() {
			fixture := &Fixture{}
			levels := make([]any, checksumDepth+1)

			for index := range levels {
				levels[index] = map[string]any{
					"price": 100.0 + float64(index),
					"qty":   10.0,
				}
			}

			row := map[string]any{
				"asks": levels,
				"bids": levels,
			}
			checksum := fixture.checksum(row)
			levels[checksumDepth].(map[string]any)["qty"] = 20.0

			Convey("Then levels beyond the checksum window do not alter the CRC", func() {
				So(fixture.checksum(row), ShouldEqual, checksum)
			})
		})
	})
}
