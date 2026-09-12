package statistic

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RegressionFit is the result of one accumulator fit.
*/
type RegressionFit struct {
	Coefficients        []float64
	CoefficientVariance []float64
	ResidualSSE         float64
	ResidualVariance    float64
	Observations        int
	Parameters          int
	Defined             bool
}

/*
RegressionRow is one design row (length p, row-major with the intercept first)
and its target value.
*/
type RegressionRow struct {
	Predictors []float64
	Target     float64
}

/*
RegressionReading fixes the facts of one incorporated row: the prequential
prediction made by the model fitted strictly on earlier rows, and the fit over
every row including this one.
*/
type RegressionReading struct {
	Prediction        float64
	PredictionDefined bool
	Fit               RegressionFit
}

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
*/
type RegressionAccumulator struct {
	err        error
	parameters int
	xtx        []float64 // p×p row-major
	xty        []float64 // p
	yty        float64
	rows       int

	fitCoefficients []float64
	fitInverse      []float64

	luScratch  []float64
	pvtScratch []int
	colScratch []float64

	rlsP       []float64
	rlsW       []float64
	rlsScratch []float64
	rlsReady   bool

	out RegressionReading
}

/*
NewRegressionAccumulator builds an empty accumulator Primitive for a model
with the given parameter count (including the intercept column).
*/
func NewRegressionAccumulator(parameters int) core.Primitive {
	if parameters < 1 {
		op := &RegressionAccumulator{}
		op.err = fmt.Errorf("%w: parameter count must be at least one", core.ErrDomain)
		return op
	}

	return &RegressionAccumulator{
		parameters:      parameters,
		xtx:             make([]float64, parameters*parameters),
		xty:             make([]float64, parameters),
		fitCoefficients: make([]float64, parameters),
		fitInverse:      make([]float64, parameters*parameters),
		luScratch:       make([]float64, parameters*parameters),
		pvtScratch:      make([]int, parameters),
		colScratch:      make([]float64, parameters),
		rlsP:            make([]float64, parameters*parameters),
		rlsW:            make([]float64, parameters),
		rlsScratch:      make([]float64, parameters),
	}
}

/*
Next scores every arriving row prequentially against the model fitted on
earlier rows, incorporates it, and hands over the reading with the refreshed
fit.
*/
func (op *RegressionAccumulator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.err != nil || op.xtx == nil {
			return
		}

		for arriving := range in {
			row := (*RegressionRow)(arriving)

			if len(row.Predictors) != op.parameters {
				op.err = fmt.Errorf("%w: design row length %d does not match parameter count %d", core.ErrShape, len(row.Predictors), op.parameters)
				return
			}

			prediction, defined := op.prequentialPredict(row.Predictors)
			op.prequentialAdd(row.Predictors, row.Target)
			op.out = RegressionReading{Prediction: prediction, PredictionDefined: defined, Fit: op.fit()}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *RegressionAccumulator) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
add incorporates one design row and its target value into the moments.
*/
func (op *RegressionAccumulator) add(predictors []float64, target float64) {
	for column := 0; column < op.parameters; column++ {
		op.xty[column] += predictors[column] * target

		for row := 0; row < op.parameters; row++ {
			op.xtx[row*op.parameters+column] += predictors[row] * predictors[column]
		}
	}

	op.yty += target * target
	op.rows++
}

/*
prequentialPredict returns the model's prediction for one design row using the
model fit on every previously incorporated row, without allocating.
*/
func (op *RegressionAccumulator) prequentialPredict(predictors []float64) (float64, bool) {
	if !op.rlsReady {
		return 0, false
	}

	prediction := 0.0

	for column := 0; column < op.parameters; column++ {
		prediction += op.rlsW[column] * predictors[column]
	}

	return prediction, true
}

/*
prequentialAdd incorporates one design row after it has been scored by
prequentialPredict.
*/
func (op *RegressionAccumulator) prequentialAdd(predictors []float64, target float64) {
	op.add(predictors, target)

	if !op.rlsReady {
		if op.rows <= op.parameters {
			return
		}

		if !op.initializeRLS() {
			return
		}

		return
	}

	denominator := 1.0

	for row := 0; row < op.parameters; row++ {
		sum := 0.0

		for column := 0; column < op.parameters; column++ {
			sum += op.rlsP[row*op.parameters+column] * predictors[column]
		}

		op.rlsScratch[row] = sum
		denominator += predictors[row] * sum
	}

	if denominator == 0 || math.IsNaN(denominator) {
		op.rlsReady = false

		return
	}

	invDenominator := 1 / denominator
	errorTerm := target

	for column := 0; column < op.parameters; column++ {
		errorTerm -= op.rlsW[column] * predictors[column]
	}

	for row := 0; row < op.parameters; row++ {
		gain := op.rlsScratch[row] * invDenominator
		op.rlsW[row] += gain * errorTerm

		for column := 0; column < op.parameters; column++ {
			op.rlsP[row*op.parameters+column] -= gain * op.rlsScratch[column]
		}
	}
}

/*
initializeRLS seeds the recursive least-squares state from the exact normal
equations at the first non-singular design.
*/
func (op *RegressionAccumulator) initializeRLS() bool {
	if !invertLU(
		op.xtx, op.rlsP, op.parameters,
		op.luScratch, op.pvtScratch, op.colScratch,
	) {
		op.rlsReady = false

		return false
	}

	if !solveLU(
		op.xtx, op.xty, op.rlsW, op.parameters,
		op.luScratch, op.pvtScratch,
	) {
		op.rlsReady = false

		return false
	}

	op.rlsReady = true

	return true
}

/*
fit solves the normal equations over the incorporated rows.
*/
func (op *RegressionAccumulator) fit() RegressionFit {
	fit := RegressionFit{
		Observations:     op.rows,
		Parameters:       op.parameters,
		ResidualVariance: math.NaN(),
		Defined:          false,
	}

	if op.rows <= op.parameters {
		return fit
	}

	if !solveLU(
		op.xtx, op.xty, op.fitCoefficients, op.parameters,
		op.luScratch, op.pvtScratch,
	) {
		return fit
	}

	fit.Coefficients = append(fit.Coefficients[:0], op.fitCoefficients...)

	sse := op.yty

	for column := 0; column < op.parameters; column++ {
		sse -= fit.Coefficients[column] * op.xty[column]
	}

	if sse < 0 {
		sse = 0
	}

	fit.ResidualSSE = sse
	fit.ResidualVariance = sse / float64(op.rows-op.parameters)
	fit.CoefficientVariance = op.coefficientVarianceFromCrossProduct(fit.ResidualVariance)
	fit.Defined = true

	return fit
}

/*
coefficientVarianceFromCrossProduct scales the diagonal of (X'X)⁻¹ by the
residual variance, reporting nil when the covariance is not identifiable.
*/
func (op *RegressionAccumulator) coefficientVarianceFromCrossProduct(residualVariance float64) []float64 {
	if math.IsNaN(residualVariance) || residualVariance < 0 {
		return nil
	}

	if !invertLU(
		op.xtx, op.fitInverse, op.parameters,
		op.luScratch, op.pvtScratch, op.colScratch,
	) {
		return nil
	}

	variances := make([]float64, op.parameters)

	for index := 0; index < op.parameters; index++ {
		diag := op.fitInverse[index*op.parameters+index]
		variance := residualVariance * diag

		if variance < 0 || math.IsNaN(variance) {
			return nil
		}

		variances[index] = variance
	}

	return variances
}
