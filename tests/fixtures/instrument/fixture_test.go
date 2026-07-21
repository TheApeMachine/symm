package instrument

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the instrument fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one instrument snapshot with assets and pairs", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				data := frame["data"].(map[string]any)
				pair := data["pairs"].([]any)[0].(map[string]any)

				So(frame["channel"], ShouldEqual, "instrument")
				So(frame["type"], ShouldEqual, "snapshot")
				So(data["assets"], ShouldNotBeEmpty)
				So(data["pairs"], ShouldNotBeEmpty)
				So(pair["status"], ShouldEqual, "online")
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate instrument update frames", func() {
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					data := frame["data"].(map[string]any)
					pairs := data["pairs"].([]any)
					pair := pairs[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(pairs, ShouldHaveLength, 1)
					So(pair["symbol"], ShouldEqual, "MATIC/USD")
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}
