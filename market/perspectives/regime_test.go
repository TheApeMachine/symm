package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRegimeConstants(t *testing.T) {
	Convey("Given regime constants", t, func() {
		Convey("It should assign distinct ordered values", func() {
			So(RegimeNone, ShouldBeLessThan, RegimeDead)
			So(RegimeDead, ShouldBeLessThan, RegimeChoppy)
			So(RegimeChoppy, ShouldBeLessThan, RegimeTrending)
			So(RegimeTrending, ShouldBeLessThan, RegimeBullish)
			So(RegimeBullish, ShouldBeLessThan, RegimeBearish)
		})
	})
}
