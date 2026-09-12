package adaptive_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestThresholdNext(t *testing.T) {
	Convey("Dispersion multipliers follow the configured coefficient", t, func() {
		for _, policy := range []struct {
			name string
			node core.Primitive
			want func(count, variance float64) float64
		}{
			{
				name: "normal",
				node: adaptive.NewThreshold(statistic.NewEstimator(), store.NewConstant(1.482602218505602)),
				want: func(count, variance float64) float64 {
					return math.Sqrt(variance) * 1.482602218505602
				},
			},
			{
				name: "chebyshev",
				node: adaptive.NewThreshold(statistic.NewEstimator(), calculus.NewSqrt()),
				want: func(count, variance float64) float64 {
					return math.Sqrt(variance) * math.Sqrt(count)
				},
			},
		} {
			Convey(policy.name, func() {
				count, mean, m2 := 0.0, 0.0, 0.0

				for _, value := range []float64{2, 2, 3, 6, 1, 9, 8, 0, 4} {
					count++
					delta := value - mean
					mean += delta / count
					m2 += delta * (value - mean)
					want := 1.0

					if count > 1 && m2 > 0 {
						want = policy.want(count, m2/(count-1))
					}

					out := tests.CollectSeq[float64](policy.node.Next(transport.NewValues(value).Next(nil)))
					So(policy.node.Error(), ShouldBeNil)
					So(out[0], ShouldAlmostEqual, want, 1e-12)
				}
			})
		}
	})
}
