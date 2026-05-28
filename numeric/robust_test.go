package numeric

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRobustScalerActivity(t *testing.T) {
	Convey("Given vectors on unrelated raw units", t, func() {
		scaler := NewRobustScaler()
		largeUnits := scaler.Activity([]float64{80, 120})
		smallUnits := scaler.Activity([]float64{0.002, 0.006})

		Convey("It should put median activity on a comparable dimensionless axis", func() {
			So(largeUnits, ShouldBeGreaterThan, 0)
			So(smallUnits, ShouldBeGreaterThan, 0)
			So(math.Abs(largeUnits-smallUnits), ShouldBeLessThan, 4)
		})
	})
}

func TestRobustScalerActivityEmpty(t *testing.T) {
	Convey("Given an empty vector", t, func() {
		scaler := NewRobustScaler()

		Convey("It should return zero activity", func() {
			So(scaler.Activity(nil), ShouldEqual, 0)
		})
	})
}

func BenchmarkRobustScalerActivity(b *testing.B) {
	values := make([]float64, 400)
	scaler := NewRobustScaler()

	for index := range values {
		values[index] = float64(index%40) * 0.01
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = scaler.Activity(values)
	}
}
