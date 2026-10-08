package statistic

import (
	"iter"
	"unsafe"

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
range.
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
	err error
	out ResidualSpanResult
}

func NewResidualSpan() core.Primitive {
	return &ResidualSpan{}
}

func (op *ResidualSpan) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*ResidualSpanInput)(arriving)
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
			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *ResidualSpan) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
