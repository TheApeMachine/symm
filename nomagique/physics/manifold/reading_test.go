package manifold

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadingIsFinite(t *testing.T) {
	Convey("Given a finite reading", t, func() {
		reading := Reading{
			PressureGradNorm: 1,
			CoherenceMag2:    0.5,
			GuidanceSpeed:    0.1,
			ViscosityProxy:   2,
		}

		So(reading.IsFinite(), ShouldBeTrue)
	})

	Convey("Given a non-finite reading", t, func() {
		reading := Reading{CoherenceMag2: math.NaN()}

		So(reading.IsFinite(), ShouldBeFalse)
	})
}
