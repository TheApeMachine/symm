package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
OLS is ordinary least squares over observation rows, composed from the
statistic layer's OLS Primitive, which owns the normal-equation solve. There
is no ridge or invented rank: when the design matrix does not have full column
rank the fit is undefined.

Each arrival is *[][]float64 of observation rows {x0, ..., x(p-1), y}: the
design columns followed by the outcome, the same target-last layout the RLS
learners use. Callers include an intercept column explicitly. It yields the
statistic layer's *[]float64 fit:

	[0] defined (1 or 0)  [1] rank         [2] observations
	[3] parameters        [4] residual SSE [5] residual variance
	[6] coefficient variance defined (1 or 0)
	[7 : 7+p]    coefficients, when defined
	[7+p : 7+2p] diagonal of cov(beta), when [6] is 1

Residual variance is NaN whenever the fit is undefined, including the empty
design. Rows of differing width, or without an outcome, report ErrShape.
*/
type OLS struct {
	*core.PrimitiveError
	solver  core.Primitive
	request [2][]float64
	out     []float64
}

func NewOLS() core.Primitive {
	return &OLS{
		PrimitiveError: core.NewPrimitiveError(),
		solver:         statistic.NewFitOLS(),
	}
}

func (op *OLS) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			observations := *(*[][]float64)(arriving)
			width := 0

			if len(observations) > 0 {
				width = len(observations[0])
			}

			op.request[0] = op.request[0][:0]
			op.request[1] = op.request[1][:0]

			for _, row := range observations {
				if width < 1 || len(row) != width {
					op.Error(fmt.Errorf("%w: algo: OLS observation rows are ragged or lack an outcome", core.ErrShape))
					return
				}

				op.request[0] = append(op.request[0], row[:width-1]...)
				op.request[1] = append(op.request[1], row[width-1])
			}

			op.out = op.out[:0]

			for pointer := range op.solver.Next(data.NewValue(op.request)) {
				op.out = append(op.out[:0], *(*[]float64)(pointer)...)
			}

			if err := op.solver.Error(); err != nil {
				op.Error(err)
				return
			}

			if len(op.out) < 7 {
				op.Error(fmt.Errorf("%w: algo: OLS solver yielded no fit", core.ErrShape))
				return
			}

			if op.out[0] != 1 {
				op.out[5] = math.NaN()
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
