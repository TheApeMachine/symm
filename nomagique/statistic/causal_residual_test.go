package statistic

import (
	"iter"
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestCausalResidual(t *testing.T) {
	Convey("Given a CausalResidual primitive", t, func() {
		residualOp := NewCausalResidual()

		feed := func(readings []MomentReading) []CausalResidualResult {
			in := func(yield func(unsafe.Pointer) bool) {
				for _, r := range readings {
					reading := r
					if !yield(unsafe.Pointer(&reading)) {
						return
					}
				}
			}

			results := make([]CausalResidualResult, 0, len(readings))
			for ptr := range residualOp.Next(iter.Seq[unsafe.Pointer](in)) {
				results = append(results, *(*CausalResidualResult)(ptr))
			}
			return results
		}

		Convey("When prior count is below MinimumPrior", func() {
			moments := Moments{}
			readings := make([]MomentReading, 0, 5)

			for i := 1; i <= 5; i++ {
				readings = append(readings, moments.Update(float64(i)))
			}

			results := feed(readings)

			Convey("ScoreScale and ZScore remain undefined (zero)", func() {
				So(len(results), ShouldEqual, 5)
				for _, res := range results {
					So(res.ScoreScale, ShouldEqual, 0)
					So(res.ZScore, ShouldEqual, 0)
				}
			})
		})

		Convey("When prior variance is negligible floating-point noise", func() {
			moments := Moments{}
			readings := make([]MomentReading, 0, 15)

			// Flat stream with 1e-16 jitter
			for i := 1; i <= 15; i++ {
				val := 100.0
				if i%2 == 0 {
					val += 1e-15
				}
				readings = append(readings, moments.Update(val))
			}

			results := feed(readings)

			Convey("ScoreScale stays zero rather than producing artificial huge z-scores", func() {
				for _, res := range results {
					So(res.ScoreScale, ShouldEqual, 0)
					So(res.ZScore, ShouldEqual, 0)
				}
			})
		})

		Convey("When prior count >= MinimumPrior with genuine dispersion", func() {
			moments := Moments{}
			readings := make([]MomentReading, 0, 12)

			for i := 1; i <= 12; i++ {
				readings = append(readings, moments.Update(float64(i)))
			}

			results := feed(readings)

			Convey("ScoreScale and ZScore are defined after MinimumPrior observations", func() {
				last := results[len(results)-1]
				So(last.Prior.Count, ShouldBeGreaterThanOrEqualTo, core.MinimumPrior)
				So(last.ScoreScale, ShouldBeGreaterThan, 0)
				expectedResidual := 12.0 - last.Baseline
				So(last.Residual, ShouldAlmostEqual, expectedResidual, 1e-9)
				So(last.ZScore, ShouldAlmostEqual, expectedResidual/last.ScoreScale, 1e-9)
				So(math.IsNaN(last.ZScore), ShouldBeFalse)
				So(math.IsInf(last.ZScore, 0), ShouldBeFalse)
			})
		})
	})
}
