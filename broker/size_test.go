package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSizeHintConviction(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the hint is zero-valued", func() {
			So(SizeHint{}.Conviction(), ShouldEqual, 1)
		})

		Convey("When a matched high-score path is set", func() {
			hint := SizeHint{
				Score:      1.0,
				Resistance: 0.05,
				PathFound:  true,
			}
			So(hint.Conviction(), ShouldBeGreaterThan, 0.55)
			So(hint.Conviction(), ShouldBeLessThanOrEqualTo, 1)
		})

		Convey("When only PathFound is set", func() {
			So(SizeHint{PathFound: true}.Conviction(), ShouldEqual, 1)
		})

		Convey("When explore has a weak score and high resistance", func() {
			hint := SizeHint{Score: 0.1, Resistance: 0.9, PathFound: false}
			So(hint.Conviction(), ShouldBeGreaterThanOrEqualTo, 0.35)
			So(hint.Conviction(), ShouldBeLessThan, 0.55)
		})
	})
}
