package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultTracker(t *testing.T) {
	Convey("Given the process-wide tracker", t, func() {
		tracker := Default()

		Convey("It should match the package default instance", func() {
			So(tracker, ShouldEqual, defaultTracker)
		})
	})
}

func TestIsToxicHelper(t *testing.T) {
	Convey("Given an unknown symbol and price", t, func() {
		now := time.Now()

		Convey("It should delegate to the default tracker", func() {
			So(IsToxic("ZZZ/ISOLATED", 123.456789, now), ShouldBeFalse)
		})
	})
}
