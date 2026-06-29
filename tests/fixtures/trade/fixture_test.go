package trade

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(testingTB *testing.T) {
	Convey("Given the trade fixture package", testingTB, func() {
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

func TestFixtureArtifacts(testingTB *testing.T) {
	Convey("Given a trade update fixture", testingTB, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When artifacts are requested", func() {
			count := 0

			for artifact := range fixture.Artifacts() {
				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "trade")
				So(scope, ShouldEqual, "update")
				count++
			}

			Convey("Then every generated frame should be converted", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})
}
