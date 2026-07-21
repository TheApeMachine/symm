package heartbeat

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the heartbeat fixture package", t, func() {
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
