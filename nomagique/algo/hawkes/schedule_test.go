package hawkes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSchedule_Ready(t *testing.T) {
	Convey("Given a fit based on sixteen events", t, func() {
		schedule := &schedule{}
		schedule.Reset(16)
		schedule.Observe(3)

		Convey("It should wait for the fourth changed event", func() {
			So(schedule.Ready(), ShouldBeFalse)
			So(schedule.Remaining(), ShouldEqual, 1)
		})

		Convey("When one more event changes", func() {
			schedule.Observe(1)

			Convey("It should invalidate the fitted epoch", func() {
				So(schedule.Ready(), ShouldBeTrue)
				So(schedule.Remaining(), ShouldEqual, 0)
			})
		})
	})
}
