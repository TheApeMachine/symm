package equation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestJointEstimator(t *testing.T) {
	Convey("Given a 3D JointEstimator", t, func() {
		estimator := &JointEstimator{}

		Convey("the first observation seeds the mean and establishes unit support", func() {
			v1 := [3]float64{10.0, 20.0, 0.05}
			estimator.Step(v1, 1000, 0)

			So(estimator.HasMean(), ShouldBeTrue)
			So(estimator.Mean(0), ShouldEqual, 10.0)
			So(estimator.Mean(1), ShouldEqual, 20.0)
			So(estimator.Mean(2), ShouldEqual, 0.05)
			So(estimator.Baseline(0), ShouldEqual, 1.0)
			So(estimator.NEff(), ShouldEqual, 1.0)
		})

		Convey("subsequent observations evaluate pre-observation residuals and update covariance", func() {
			v1 := [3]float64{10.0, 20.0, 0.05}
			estimator.Step(v1, 1000, 0)

			v2 := [3]float64{12.0, 22.0, 0.07}
			estimator.Step(v2, 1001, 0)

			So(estimator.Residual(0), ShouldAlmostEqual, 2.0, 1e-9)
			So(estimator.Residual(1), ShouldAlmostEqual, 2.0, 1e-9)
			So(estimator.Residual(2), ShouldAlmostEqual, 0.02, 1e-9)
			So(estimator.Ratio(0), ShouldAlmostEqual, math.Exp(2.0), 1e-9)
			So(estimator.NEff(), ShouldBeGreaterThan, 1.0)
		})
	})
}
