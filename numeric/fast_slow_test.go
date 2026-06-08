package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFastSlowRate(t *testing.T) {
	Convey("Given a silent slow window and a recent spike", t, func() {
		samples := []float64{0, 0, 0, 0, 0, 10, 10, 10}

		rate := FastSlowRate(samples, 3, 1e-6)

		Convey("It should exceed unity", func() {
			So(rate, ShouldBeGreaterThan, 1.0)
		})
	})

	Convey("Given fewer samples than the fast window", t, func() {
		rate := FastSlowRate([]float64{1, 2}, 3, 1e-6)

		Convey("It should return the neutral ratio", func() {
			So(rate, ShouldEqual, 1.0)
		})
	})
}

func TestInvertedFastSlowRate(t *testing.T) {
	Convey("Given tightening spreads", t, func() {
		spreads := []float64{0.5, 0.5, 0.5, 0.5, 0.2, 0.2, 0.2}

		compression := InvertedFastSlowRate(spreads, 3, 1e-6)

		Convey("It should exceed unity", func() {
			So(compression, ShouldBeGreaterThan, 1.0)
		})
	})
}

func TestFastSlowRatioNegativeSample(t *testing.T) {
	Convey("Given a negative volume sample", t, func() {
		ratio := NewFastSlowRatio(3, 1e-6)

		_, err := ratio.Next(0, -1.0)

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestFastSlowRatioNext(t *testing.T) {
	Convey("Given a FastSlowRatio dynamic", t, func() {
		ratio := NewFastSlowRatio(3, 1e-6)

		out, err := ratio.Next(0, []float64{1, 1, 1, 4, 4, 4}...)

		Convey("It should implement Dynamic without error", func() {
			So(err, ShouldBeNil)
			So(out, ShouldBeGreaterThan, 1.0)
		})
	})
}

func BenchmarkFastSlowRate(b *testing.B) {
	samples := make([]float64, 128)

	for index := range samples {
		samples[index] = float64(index%5) + 1
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = FastSlowRate(samples, 3, 1e-6)
	}
}
