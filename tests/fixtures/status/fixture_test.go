package status

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(testingTB *testing.T) {
	Convey("Given the status fixture package", testingTB, func() {
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

func TestFixtureArtifacts(testingTB *testing.T) {
	Convey("Given a status update fixture", testingTB, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When artifacts are requested", func() {
			count := 0

			for artifact := range fixture.Artifacts() {
				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "status")
				So(scope, ShouldEqual, "update")
				count++
			}

			Convey("Then every generated frame should be converted", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})
}
