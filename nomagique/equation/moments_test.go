package equation

import (
	. "github.com/smartystreets/goconvey/convey"
	"math"
	"testing"
)

func TestMomentsUpdate(t *testing.T) {
	Convey("A multi-step sequence has exact prior moments and Bessel-corrected variance", t, func() {
		var moments Moments
		first := moments.Update(-2)
		So(first.Prior.Count, ShouldEqual, 0)
		So(first.VarianceDefined, ShouldBeFalse)
		So(math.IsNaN(first.Variance), ShouldBeTrue)
		moments.Update(0)
		third := moments.Update(2)
		So(third.Prior.Mean, ShouldEqual, -1)
		So(third.Prior.Count, ShouldEqual, 2)
		last := moments.Update(4)
		So(last.Count, ShouldEqual, 4)
		So(last.Mean, ShouldEqual, 1)
		So(last.M2, ShouldEqual, 20)
		So(last.Variance, ShouldAlmostEqual, 20.0/3)
		So(first.Mean, ShouldEqual, -2)
	})
}

func TestMomentsShed(t *testing.T) {
	Convey("Shedding preserves corrected dispersion and the mean", t, func() {
		moments := Moments{Count: 4, Mean: 1, M2: 20}
		moments.Shed(0.5)
		So(moments.Count, ShouldEqual, 2)
		So(moments.Mean, ShouldEqual, 1)
		So(moments.M2/(moments.Count-1), ShouldAlmostEqual, 20.0/3)
		Convey("the two-sample floor admits the next real observation", func() {
			moments.Shed(0.5)
			reading := moments.Update(4)
			So(reading.Prior.Count, ShouldEqual, 2)
			So(reading.Mean, ShouldEqual, 2)
			So(reading.M2, ShouldAlmostEqual, 38.0/3)
		})
	})
	Convey("Non-contraction policies leave moments unchanged", t, func() {
		for _, retain := range []float64{-1, 0, 1, 2} {
			moments := Moments{Count: 4, Mean: 1, M2: 20}
			moments.Shed(retain)
			So(moments, ShouldResemble, Moments{Count: 4, Mean: 1, M2: 20})
		}
	})
}

func BenchmarkMomentsUpdate(b *testing.B) {
	var moments Moments
	index := 0
	b.ReportAllocs()
	for b.Loop() {
		moments.Update(float64(index % 7))
		index++
	}
}
