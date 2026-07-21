package status

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the status fixture package", t, func() {
		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate status update frames", func() {
				var previous uint64
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					row := frame["data"].([]any)[0].(map[string]any)

					So(frame["channel"], ShouldEqual, "status")
					So(frame["type"], ShouldEqual, "update")
					So(row["system"], ShouldEqual, "online")
					So(uint64(row["connection_id"].(float64)), ShouldBeGreaterThanOrEqualTo, previous)
					previous = uint64(row["connection_id"].(float64))
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}
