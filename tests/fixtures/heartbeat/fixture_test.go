package heartbeat

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(testingTB *testing.T) {
	Convey("Given the heartbeat fixture package", testingTB, func() {
		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate heartbeat frames", func() {
				count := 0

				for payload := range fixture.Generate() {
					So(strings.TrimSpace(string(payload)), ShouldEqual, `{"channel":"heartbeat"}`)
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestFixtureArtifacts(testingTB *testing.T) {
	Convey("Given a heartbeat update fixture", testingTB, func() {
		fixture := NewFixture(UPDATE, 2)

		Convey("When artifacts are requested", func() {
			count := 0

			for artifact := range fixture.Artifacts() {
				role, roleErr := artifact.Role()

				So(roleErr, ShouldBeNil)
				So(role, ShouldEqual, "heartbeat")
				count++
			}

			Convey("Then every generated frame should be converted", func() {
				So(count, ShouldEqual, 2)
			})
		})
	})
}
