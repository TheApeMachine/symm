package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LogGradientInput is a natural-parameter gradient plus the parameters it was
taken at.
*/
type LogGradientInput struct {
	Parameters
	Gradient []float64
}

/*
LogGradient applies the chain rule for log(mu), log(beta), and log branching
coordinates.
*/
type LogGradient struct {
	core.Base[LogGradientInput, []float64]
}

func NewLogGradient() *LogGradient {
	return &LogGradient{}
}

func (op *LogGradient) Next(
	in iter.Seq[core.Primitive[LogGradientInput, LogGradientInput]],
) iter.Seq[core.Primitive[[]float64, []float64]] {
	return func(yield func(core.Primitive[[]float64, []float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			g := input.Gradient

			if len(g) < 7 {
				op.Error(core.ErrShape)
				continue
			}

			logg := []float64{
				g[0] * input.MuX,
				g[1] * input.MuY,
				g[2] * input.AlphaXX,
				g[3] * input.AlphaXY,
				g[4] * input.AlphaYX,
				g[5] * input.AlphaYY,
				g[6]*input.Beta + g[2]*input.AlphaXX + g[3]*input.AlphaXY + g[4]*input.AlphaYX + g[5]*input.AlphaYY,
			}

			if !yield(op.Carrier(logg)) {
				return
			}
		}
	}
}
