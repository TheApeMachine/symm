package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestProjectScalar(t *testing.T) {
	Convey("Given a scalar projection in a Derived chain", t, func() {
		chain := NewDerived(WithDynamics(
			NewProjectScalar(func(_ float64, values []float64) float64 {
				return values[1] * (1 + 2*values[0]) / values[2]
			}),
			adaptive.NewEMA(0),
		))

		out, err := chain.Push(0.5, 0.02, 1.0)

		Convey("It should fuse without allocating a remap slice", func() {
			So(err, ShouldBeNil)
			So(out, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkProjectScalarPush(b *testing.B) {
	pipe := NewClassed(
		adaptive.NewClassifier(
			[]float64{-0.30, 0.40, 2.00},
			[]float64{0, 1, 2, 3},
			[]string{"divergent_stress", "stochastic_noise", "decoupled_alpha", "systemic_herd"},
		),
		NewProjectScalar(func(_ float64, values []float64) float64 {
			return values[1] * (1 + 2*values[0]) / values[2]
		}),
		adaptive.NewEMA(0),
		adaptive.NewSigmaClamp(3, 8, 0.0625),
	)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = pipe.Push(0.85, 0.02, 1.0)
		_ = pipe.Confidence()
	}
}

func BenchmarkProjectVectorPush(b *testing.B) {
	pipe := NewClassed(
		adaptive.NewClassifier(
			[]float64{-0.30, 0.40, 2.00},
			[]float64{0, 1, 2, 3},
			[]string{"divergent_stress", "stochastic_noise", "decoupled_alpha", "systemic_herd"},
		),
		NewProject(func(_ float64, values []float64) []float64 {
			return []float64{values[1] * (1 + 2*values[0]) / values[2]}
		}),
		adaptive.NewEMA(0),
		adaptive.NewSigmaClamp(3, 8, 0.0625),
	)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = pipe.Push(0.85, 0.02, 1.0)
		_ = pipe.Confidence()
	}
}
