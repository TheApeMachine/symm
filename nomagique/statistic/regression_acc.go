package statistic

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
RegressionAccumulator owns the normal-equation moments (X'X, X'y, y'y) of a
linear regression incrementally across prequential steps, so each step's fit
costs O(p²) instead of refitting the full design from scratch. The intercept
is an explicit column of ones supplied by the caller.

Rank deficiency is explicit: the cross-product matrix is singular exactly
when the design is rank-deficient, and the fit is then undefined. No ridge,
dropped coordinate, or fabricated zero is substituted. Normal equations are
less numerically robust than a full SVD refit for ill-conditioned designs,
which is why rank is checked by the singularity of X'X; the mathematical
results match ordinary least squares for well-conditioned designs.

Each arrival is one design row as *[]float64 of length p+1: the p predictors
(row-major, intercept first) followed by the target value. The row is scored
prequentially by the recursive least-squares model fitted strictly on earlier
rows, then incorporated. It yields one *[]float64 reading:

	[0] prediction        [1] prediction defined (1 or 0)
	[2] fit defined       [3] observations      [4] parameters
	[5] residual SSE      [6] residual variance (NaN while undefined)
	[7] coefficient variance defined (1 or 0)
	[8 : 8+p]    coefficients, when the fit is defined
	[8+p : 8+2p] diagonal of sigma² (X'X)⁻¹, when [7] is 1

The yielded slice is reused by the next arrival.
*/
type RegressionAccumulator struct {
	*core.PrimitiveError
	parameters int
	xtx        []float64
	xty        []float64
	yty        float64
	rows       int
	solve      core.Primitive
	invert     core.Primitive
	rlsP       []float64
	rlsW       []float64
	rlsScratch []float64
	rlsReady   bool
	out        []float64
}

/*
NewRegressionAccumulator builds an empty accumulator Primitive for a model
with the given parameter count (including the intercept column).
*/
func NewRegressionAccumulator(parameters int) core.Primitive {
	op := &RegressionAccumulator{
		PrimitiveError: core.NewPrimitiveError(),
		solve:          NewSolveLU(),
		invert:         NewInvertLU(),
	}

	if parameters < 1 {
		op.Error(fmt.Errorf("%w: parameter count must be at least one", core.ErrDomain))
		return op
	}

	op.parameters = parameters
	op.xtx = make([]float64, parameters*parameters)
	op.xty = make([]float64, parameters)
	op.rlsP = make([]float64, parameters*parameters)
	op.rlsW = make([]float64, parameters)
	op.rlsScratch = make([]float64, parameters)
	op.out = make([]float64, 0, 8+2*parameters)

	return op
}

/*
Next scores every arriving row prequentially against the model fitted on
earlier rows, incorporates it, and hands over the reading with the refreshed
fit.
*/
func (op *RegressionAccumulator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil || op.xtx == nil {
			return
		}

		p := op.parameters

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			row := *(*[]float64)(arriving)

			if len(row) != p+1 {
				op.Error(fmt.Errorf("%w: design row length %d does not match parameter count %d", core.ErrShape, len(row)-1, p))
				return
			}

			predictors, target := row[:p], row[p]
			op.out = append(op.out[:0], 0, 0, 0, float64(0), float64(p), 0, math.NaN(), 0)

			if op.rlsReady {
				prediction := 0.0

				for column := 0; column < p; column++ {
					prediction += op.rlsW[column] * predictors[column]
				}

				op.out[0] = prediction
				op.out[1] = 1
			}

			for column := 0; column < p; column++ {
				op.xty[column] += predictors[column] * target

				for index := 0; index < p; index++ {
					op.xtx[index*p+column] += predictors[index] * predictors[column]
				}
			}

			op.yty += target * target
			op.rows++
			op.out[3] = float64(op.rows)

			if op.rlsReady {
				denominator := 1.0

				for index := 0; index < p; index++ {
					sum := 0.0

					for column := 0; column < p; column++ {
						sum += op.rlsP[index*p+column] * predictors[column]
					}

					op.rlsScratch[index] = sum
					denominator += predictors[index] * sum
				}

				if denominator == 0 || math.IsNaN(denominator) {
					op.rlsReady = false
				}

				if op.rlsReady {
					invDenominator := 1 / denominator
					errorTerm := target

					for column := 0; column < p; column++ {
						errorTerm -= op.rlsW[column] * predictors[column]
					}

					for index := 0; index < p; index++ {
						gain := op.rlsScratch[index] * invDenominator
						op.rlsW[index] += gain * errorTerm

						for column := 0; column < p; column++ {
							op.rlsP[index*p+column] -= gain * op.rlsScratch[column]
						}
					}
				}
			} else if op.rows > p {
				// Seed recursive least squares from the exact normal equations
				// at the first non-singular design.
				var inverse []float64

				for pointer := range op.invert.Next(data.NewValue(op.xtx)) {
					inverse = *(*[]float64)(pointer)
				}

				if err := op.invert.Error(); err != nil {
					op.Error(err)
					return
				}

				var weights []float64

				if len(inverse) == p*p {
					copy(op.rlsP, inverse)

					for pointer := range op.solve.Next(data.NewValue([2][]float64{op.xtx, op.xty})) {
						weights = *(*[]float64)(pointer)
					}

					if err := op.solve.Error(); err != nil {
						op.Error(err)
						return
					}
				}

				op.rlsReady = len(weights) == p
				copy(op.rlsW, weights)
			}

			var coefficients []float64

			if op.rows > p {
				for pointer := range op.solve.Next(data.NewValue([2][]float64{op.xtx, op.xty})) {
					coefficients = *(*[]float64)(pointer)
				}

				if err := op.solve.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			if len(coefficients) == p {
				sse := op.yty

				for column := 0; column < p; column++ {
					sse -= coefficients[column] * op.xty[column]
				}

				if sse < 0 {
					sse = 0
				}

				residualVariance := sse / float64(op.rows-p)
				op.out[2] = 1
				op.out[5] = sse
				op.out[6] = residualVariance
				op.out = append(op.out, coefficients...)

				var inverse []float64

				if !math.IsNaN(residualVariance) && residualVariance >= 0 {
					for pointer := range op.invert.Next(data.NewValue(op.xtx)) {
						inverse = *(*[]float64)(pointer)
					}

					if err := op.invert.Error(); err != nil {
						op.Error(err)
						return
					}
				}

				identifiable := len(inverse) == p*p

				for index := 0; identifiable && index < p; index++ {
					variance := residualVariance * inverse[index*p+index]
					identifiable = variance >= 0 && !math.IsNaN(variance)
				}

				for index := 0; identifiable && index < p; index++ {
					op.out = append(op.out, residualVariance*inverse[index*p+index])
				}

				if identifiable {
					op.out[7] = 1
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
