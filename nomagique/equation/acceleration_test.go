package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAcceleration(t *testing.T) {
	Convey("Given a quantity-clocked positive series", t, func() {
		rr := NewRenewalRate()

		// First step at t=0
		rr.Step(types.Scalar(2), types.Scalar(100), 0)
		So(rr.Closed(), ShouldBeFalse)

		// Second step at t=1: accumulated = 4 >= 2, dt = 1
		rr.Step(types.Scalar(2), types.Scalar(100), 1)
		So(rr.Closed(), ShouldBeTrue)
		So(float64(rr.Rate()), ShouldEqual, 4.0)
		So(float64(rr.Maturity()), ShouldEqual, 0.5)

		// Third step at t=2: accumulated = 2 >= 2, dt = 1
		rr.Step(types.Scalar(2), types.Scalar(110), 2)
		So(rr.Closed(), ShouldBeTrue)
		So(float64(rr.Rate()), ShouldEqual, 2.0)
		So(float64(rr.Change()), ShouldAlmostEqual, math.Log(1.1), 1e-12)
		So(float64(rr.Maturity()), ShouldAlmostEqual, 2.0/3.0, 1e-12)
	})
}
