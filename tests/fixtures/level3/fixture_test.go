package level3

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(testingTB *testing.T) {
	Convey("Given the level3 fixture package", testingTB, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one level3 snapshot without order events", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				row := frame["data"].([]any)[0].(map[string]any)
				bid := row["bids"].([]any)[0].(map[string]any)

				So(frame["channel"], ShouldEqual, "level3")
				So(frame["type"], ShouldEqual, "snapshot")
				So(row["bids"], ShouldNotBeEmpty)
				So(bid["event"], ShouldBeNil)
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate an ordered level3 event sequence", func() {
				checksum := uint64(0)
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					row := frame["data"].([]any)[0].(map[string]any)
					ask := row["asks"].([]any)[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(ask["event"], ShouldEqual, "delete")
					So(uint64(row["checksum"].(float64)), ShouldBeGreaterThan, checksum)
					checksum = uint64(row["checksum"].(float64))
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestFixtureArtifacts(testingTB *testing.T) {
	Convey("Given a level3 update fixture", testingTB, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When artifacts are requested", func() {
			count := 0

			for artifact := range fixture.Artifacts() {
				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "level3")
				So(scope, ShouldEqual, "update")
				count++
			}

			Convey("Then every generated frame should be converted", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})
}
