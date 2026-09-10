package adaptive_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestThresholdNext(t *testing.T) {
	Convey("Dispersion multipliers follow the configured coefficient", t, func() {
		for _, policy := range []struct {
			name string
			node *adaptive.Threshold
			want func(count, variance float64) float64
		}{
			{
				name: "predictive",
				node: adaptive.NewThreshold(equation.NewWelford(), equation.NewPredictiveInflation()),
				want: func(count, variance float64) float64 {
					return math.Sqrt(variance) * math.Sqrt(1+1/count)
				},
			},
			{
				name: "normal",
				node: adaptive.NewThreshold(equation.NewWelford(), store.NewConstant[float64, float64](1.482602218505602)),
				want: func(count, variance float64) float64 {
					return math.Sqrt(variance) * 1.482602218505602
				},
			},
			{
				name: "chebyshev",
				node: adaptive.NewThreshold(equation.NewWelford(), calculus.NewSqrt[float64]()),
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

					got, err := transport.Evaluate(policy.node, transport.Values(value))
					So(err, ShouldBeNil)
					So(got, ShouldAlmostEqual, want)
				}
			})
		}
	})
}
