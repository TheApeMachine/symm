package adaptive

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEWMomentsUpdate(t *testing.T) {
	Convey("Given a fresh EWMoments tracker", t, func() {
		moments := &EWMoments{}

		err := moments.Update(2.0, 0.2)
		So(err, ShouldBeNil)

		Convey("It should seed mean on the first observation", func() {
			So(moments.Observations(), ShouldEqual, 1)
			So(moments.Mean(), ShouldEqual, 2.0)
			So(moments.VarianceEWMA(), ShouldEqual, 0)
		})

		Convey("It should reject invalid alpha", func() {
			err := moments.Update(1.0, 0)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given repeated observations", t, func() {
		moments := &EWMoments{}
		alpha := 0.3

		for _, value := range []float64{1.0, 3.0, 2.0, 4.0} {
			err := moments.Update(value, alpha)
			So(err, ShouldBeNil)
		}

		Convey("It should track a non-zero EW variance", func() {
			So(moments.Observations(), ShouldEqual, 4)
			So(moments.Mean(), ShouldBeGreaterThan, 0)
			So(moments.VarianceEWMA(), ShouldBeGreaterThan, 0)
		})

		Convey("It should reset to empty state", func() {
			moments.Reset()
			So(moments.Observations(), ShouldEqual, 0)
			So(moments.Mean(), ShouldEqual, 0)
			So(moments.VarianceEWMA(), ShouldEqual, 0)
		})
	})
}

func BenchmarkEWMomentsUpdate(b *testing.B) {
	moments := &EWMoments{}
	alpha := 0.15

	for b.Loop() {
		value := math.Sin(float64(moments.Observations()))
		_ = moments.Update(value, alpha)
	}
}
