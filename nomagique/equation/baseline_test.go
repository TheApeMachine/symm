package equation

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

// MinimumSupportForBaseline is the support below which a shed baseline would
// be too thin to score against at all.
const MinimumSupportForBaseline = 30

func TestCausalBaseline(t *testing.T) {
	Convey("Given a CausalResidual equation", t, func() {
		residualEq := &CausalResidual{}

		Convey("the first observation sets baseline to itself and emits zero residual", func() {
			residual := residualEq.Step(types.Scalar(100.0))
			So(residual, ShouldEqual, 0.0)
			So(residualEq.Baseline(), ShouldEqual, 100.0)
			So(residualEq.Count(), ShouldEqual, 1.0)
		})

		Convey("subsequent observations evaluate residuals against running mean", func() {
			residualEq.Step(types.Scalar(100.0))
			residual := residualEq.Step(types.Scalar(110.0))
			So(residual, ShouldNotEqual, 0.0)
			So(residualEq.Mean(), ShouldEqual, 105.0)
			So(residualEq.Count(), ShouldEqual, 2.0)
		})
	})
}

/*
regimeTape returns an hour-shaped stream: several distinct activity levels in
sequence, as one instrument's throughput actually moves through a session.
*/
func regimeTape(rng *rand.Rand, count int) []float64 {
	levels := []float64{0.02, 0.14, 0.05, 0.22, 0.08, 0.17, 0.04, 0.11}
	tape := make([]float64, 0, count)

	for index := 0; len(tape) < count; index++ {
		level := levels[index%len(levels)]

		for range 125 {
			if len(tape) == count {
				break
			}

			tape = append(tape, level+rng.NormFloat64()*0.02)
		}
	}

	return tape
}

/*
TestCausalBaselineForgetsAcrossRegimes pins the property the estimator exists
to provide: a departure from the CURRENT regime must be measurable.

An estimator that never forgets converges on the moments pooled across every
regime of the session. Its dispersion then spans the differences BETWEEN
regimes, which are far larger than anything happening inside one, so no
departure from the level now in force can reach it. The failure is silent: the
estimator reports full support and near-full maturity for a baseline that
cannot answer the only question asked of it.
*/
func TestCausalBaselineForgetsAcrossRegimes(t *testing.T) {
	Convey("Given a baseline observing an hour of shifting regimes", t, func() {
		rng := rand.New(rand.NewSource(7))
		tape := regimeTape(rng, 996)

		residualEq := &CausalResidual{}
		var pooled adaptive.WelfordEngine

		for _, sample := range tape {
			residualEq.Step(types.Scalar(sample))
			pooled.Update(sample)
		}

		Convey("its dispersion is narrower than the pooled-session dispersion", func() {
			So(
				float64(residualEq.Dispersion()),
				ShouldBeLessThan,
				pooled.Dispersion(),
			)
		})

		Convey("its support reflects the current regime, not the whole session", func() {
			So(residualEq.Count(), ShouldBeLessThan, pooled.Count())
			So(residualEq.Count(), ShouldBeGreaterThan, MinimumSupportForBaseline)
		})

		Convey("a sustained departure from the current level clears its own noise", func() {
			// SNR is divergence^2/noise_variance, so a z-score above 1 is the
			// point at which a reading stands above the estimator's own noise
			// rather than being indistinguishable from it.
			elevated := 0.0

			for range 6 {
				residualEq.Step(types.Scalar(0.18 + rng.NormFloat64()*0.02))

				if math.Abs(float64(residualEq.ZScore())) > 1 {
					elevated++
				}
			}

			So(elevated, ShouldBeGreaterThan, 3)
		})
	})
}
