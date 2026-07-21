package candles

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the candles fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one OHLC snapshot frame with history", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				So(frame["channel"], ShouldEqual, "ohlc")
				So(frame["type"], ShouldEqual, "snapshot")
				So(len(frame["data"].([]any)), ShouldBeGreaterThan, 1)
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate an ordered candle sequence", func() {
				trades := uint64(0)
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					row := frame["data"].([]any)[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(uint64(row["trades"].(float64)), ShouldBeGreaterThan, trades)
					trades = uint64(row["trades"].(float64))
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}
