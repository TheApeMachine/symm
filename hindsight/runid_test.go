package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRunID(t *testing.T) {
	Convey("Given process run identity generation", t, func() {
		startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

		Convey("Two runs in the same instant generate distinct IDs", func() {
			first, err := NewRunID(startedAt)
			So(err, ShouldBeNil)

			second, err := NewRunID(startedAt)
			So(err, ShouldBeNil)

			So(first, ShouldNotEqual, second)
		})

		Convey("A zero start instant is rejected", func() {
			id, err := NewRunID(time.Time{})
			So(id, ShouldEqual, RunID(""))
			So(err, ShouldNotBeNil)
		})
	})
}
