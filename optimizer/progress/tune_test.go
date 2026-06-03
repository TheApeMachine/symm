package progress

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTuneProgressInterval(t *testing.T) {
	Convey("Given workload sizes", t, func() {
		Convey("It should return one for trivial totals", func() {
			So(TuneProgressInterval(1), ShouldEqual, 1)
			So(TuneProgressInterval(8), ShouldEqual, 1)
		})

		Convey("It should target roughly twenty updates on large totals", func() {
			So(TuneProgressInterval(432), ShouldEqual, 21)
			So(TuneProgressInterval(19872), ShouldEqual, 512)
		})
	})
}

func TestTuneProgressReporter(t *testing.T) {
	Convey("Given a progress reporter", t, func() {
		reporter := NewTuneProgressReporter(100)

		Convey("It should always log the first and last steps", func() {
			So(reporter.ShouldLog(1), ShouldBeTrue)
			So(reporter.ShouldLog(100), ShouldBeTrue)
		})

		Convey("It should log on interval boundaries", func() {
			So(reporter.ShouldLog(5), ShouldBeTrue)
			So(reporter.ShouldLog(7), ShouldBeFalse)
		})

		Convey("It should log again after the minimum spacing elapses", func() {
			reporter.MarkLogged()
			reporter.lastLogged = time.Now().Add(-tuneProgressMinSpacing)

			So(reporter.ShouldLog(2), ShouldBeTrue)
		})
	})
}
