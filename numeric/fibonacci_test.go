package numeric

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFibWindows(t *testing.T) {
	Convey("Given Fibonacci window constants", t, func() {
		Convey("It should expose the expected window sizes", func() {
			So(FibWindows, ShouldResemble, []int{3, 5, 8, 13, 21})
		})

		Convey("It should derive inverse-scale weights that sum to one", func() {
			So(len(FibWeights), ShouldEqual, len(FibWindows))

			var total float64

			for _, weight := range FibWeights {
				total += weight
			}

			So(total, ShouldAlmostEqual, 1, 1e-12)

			for index, window := range FibWindows {
				expected := (1 / float64(window)) / inverseScaleSum()
				So(FibWeights[index], ShouldAlmostEqual, expected, 1e-12)
			}
		})
	})
}

func inverseScaleSum() float64 {
	var total float64

	for _, window := range FibWindows {
		total += 1 / float64(window)
	}

	return total
}

func BenchmarkFibWeightsAccess(b *testing.B) {
	for b.Loop() {
		_ = FibWeights[len(FibWeights)-1] * math.Sqrt(float64(FibWindows[0]))
	}
}
