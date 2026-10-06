package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
OLS owns one ordinary least squares fit as a Primitive. The caller owns the
design: an intercept column of ones is included only when the model requires
it. Rank deficiency is an explicit state: when the design matrix does not have
full column rank the fit is undefined, and no ridge, dropped coordinate, or
fabricated zero is substituted. Undefined is not zero.

Each arrival is *[2][]float64 {x, y}: x is the row-major n×p design matrix and
y holds the n targets, so p = len(x) / len(y). It yields one *[]float64 fit:

	[0] defined (1 or 0)  [1] rank         [2] observations
	[3] parameters        [4] residual SSE [5] residual variance
	[6] coefficient variance defined (1 or 0)
	[7 : 7+p]    coefficients, one per design column, when defined
	[7+p : 7+2p] diagonal of cov(beta) = sigma² (X'X)⁻¹, when [6] is 1

Residual variance is SSE/(n-p) when the fit is defined, otherwise NaN when
n <= p or the design is singular.
*/
type OLS struct {
	*core.PrimitiveError
	solve  core.Primitive
	invert core.Primitive
	xtx    []float64
	xty    []float64
	out    []float64
}

/*
NewFitOLS instantiates the ordinary least squares Primitive.
*/
func NewFitOLS() *OLS {
	return &OLS{
		PrimitiveError: core.NewPrimitiveError(),
		solve:          NewSolveLU(),
		invert:         NewInvertLU(),
	}
}

/*
Next fits every arriving design and hands over the resulting fit.
*/
func (op *OLS) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			request := (*[2][]float64)(arriving)
			x, y := request[0], request[1]
			n := len(y)
			p := 0

			if n > 0 {
				p = len(x) / n
			}

			if n > 0 && len(x) != n*p {
				op.Error(core.ErrShape)
				return
			}

			op.out = append(op.out[:0], 0, 0, 0, 0, 0, 0, 0)

			if p > 0 {
				op.out[2] = float64(n)
				op.out[3] = float64(p)
				op.out[5] = math.NaN()
			}

			if p < 1 || n <= p {
				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			if cap(op.xtx) < p*p {
				op.xtx = make([]float64, p*p)
				op.xty = make([]float64, p)
			}

			op.xtx = op.xtx[:p*p]
			op.xty = op.xty[:p]
			clear(op.xtx)
			clear(op.xty)
			yty := 0.0

			for row := 0; row < n; row++ {
				rowOffset := row * p
				yVal := y[row]
				yty += yVal * yVal

				for column := 0; column < p; column++ {
					op.xty[column] += x[rowOffset+column] * yVal

					for k := 0; k <= column; k++ {
						op.xtx[column*p+k] += x[rowOffset+column] * x[rowOffset+k]
					}
				}
			}

			for column := 0; column < p; column++ {
				for k := 0; k < column; k++ {
					op.xtx[k*p+column] = op.xtx[column*p+k]
				}
			}

			var coefficients []float64

			for pointer := range op.solve.Next(data.NewValue([2][]float64{op.xtx, op.xty})) {
				coefficients = *(*[]float64)(pointer)
			}

			if err := op.solve.Error(); err != nil {
				op.Error(err)
				return
			}

			if len(coefficients) == p {
				sse := yty

				for column := 0; column < p; column++ {
					sse -= coefficients[column] * op.xty[column]
				}

				if sse < 0 {
					sse = 0
				}

				residualVariance := sse / float64(n-p)
				op.out[0] = 1
				op.out[1] = float64(p)
				op.out[4] = sse
				op.out[5] = residualVariance
				op.out = append(op.out, coefficients...)

				var inverse []float64

				for pointer := range op.invert.Next(data.NewValue(op.xtx)) {
					inverse = *(*[]float64)(pointer)
				}

				if err := op.invert.Error(); err != nil {
					op.Error(err)
					return
				}

				if len(inverse) == p*p {
					op.out[6] = 1

					for index := 0; index < p; index++ {
						op.out = append(op.out, residualVariance*inverse[index*p+index])
					}
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
