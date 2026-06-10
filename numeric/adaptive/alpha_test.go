package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAlphaFromSurprise(t *testing.T) {
	Convey("Given surprise-modulated alpha bounds", t, func() {
		Convey("It should stay at alphaMin when surprise is normal", func() {
			So(AlphaFromSurprise(1.0, 0.01, 0.25), ShouldEqual, 0.01)
		})

		Convey("It should reach alphaMax when surprise spikes", func() {
			So(AlphaFromSurprise(2.0, 0.01, 0.25), ShouldEqual, 0.25)
		})

		Convey("It should interpolate between bounds", func() {
			So(AlphaFromSurprise(1.5, 0.01, 0.25), ShouldEqual, 0.13)
		})
	})
}
