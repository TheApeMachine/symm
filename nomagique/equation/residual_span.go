package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ResidualSpanInput is the running calibration range and the arriving residual.
*/
type ResidualSpanInput struct {
	Count    float64
	Minimum  float64
	Maximum  float64
	Residual float64
}

/*
ResidualSpanResult is the source's state counter and the observed residual
range. Count stays at one until the first distinct residual, then counts every
update.
*/
type ResidualSpanResult struct {
	Count   float64
	Minimum float64
	Maximum float64
	Span    float64
}

/*
ResidualSpan reproduces the supplied calibration range update.
*/
type ResidualSpan struct {
	core.Base[ResidualSpanInput, ResidualSpanResult]
}

func NewResidualSpan() *ResidualSpan {
	return &ResidualSpan{}
}

func (op *ResidualSpan) Next(
	in iter.Seq[core.Primitive[ResidualSpanInput, ResidualSpanInput]],
) iter.Seq[core.Primitive[ResidualSpanResult, ResidualSpanResult]] {
	return func(yield func(core.Primitive[ResidualSpanResult, ResidualSpanResult]) bool) {
		for arriving := range in {
			input := arriving.Read()
			result := ResidualSpanResult{
				Count:   input.Count,
				Minimum: input.Minimum,
				Maximum: input.Maximum,
			}

			if input.Count == 0 {
				result.Minimum = input.Residual
				result.Maximum = input.Residual
				result.Count = 1
			}

			if result.Count > 1 {
				if input.Residual < result.Minimum {
					result.Minimum = input.Residual
				}

				if input.Residual > result.Maximum {
					result.Maximum = input.Residual
				}

				result.Count = input.Count + 1
			}

			if result.Count == 1 && input.Residual != result.Minimum {
				if input.Residual < result.Minimum {
					result.Minimum = input.Residual
				}

				if input.Residual > result.Maximum {
					result.Maximum = input.Residual
				}

				result.Count = 2
			}

			result.Span = result.Maximum - result.Minimum

			if !yield(op.Carrier(result)) {
				return
			}
		}
	}
}
