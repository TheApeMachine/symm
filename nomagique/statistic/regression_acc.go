package statistic

import (
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
	*core.PrimitiveError

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
func NewRegressionAccumulator(parameters int) *RegressionAccumulator {
	if parameters < 1 {
		op := &RegressionAccumulator{PrimitiveError: core.NewPrimitiveError()}
		op.Error(fmt.Errorf("%w: parameter count must be at least one", core.ErrDomain))
		return op
	}

	return &RegressionAccumulator{PrimitiveError: core.NewPrimitiveError(), parameters: parameters,
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
func (regressionAccumulator *RegressionAccumulator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if regressionAccumulator.
			Error() !=
			nil || regressionAccumulator.xtx == nil {
			return
		}

		for arriving := range in {
			row := (*RegressionRow)(arriving)

			if len(row.Predictors) != regressionAccumulator.parameters {
				regressionAccumulator.Error(fmt.Errorf("%w: design row length %d does not match parameter count %d", core.ErrShape, len(row.Predictors), regressionAccumulator.parameters))
				return
			}

			prediction, defined := regressionAccumulator.prequentialPredict(row.Predictors)
			regressionAccumulator.prequentialAdd(row.Predictors, row.Target)
			regressionAccumulator.out = RegressionReading{Prediction: prediction, PredictionDefined: defined, Fit: regressionAccumulator.fit()}

			if !yield(unsafe.Pointer(&regressionAccumulator.out)) {
				return
			}
		}
	}
}

/*
add incorporates one design row and its target value into the moments.
*/
func (regressionAccumulator *RegressionAccumulator) add(predictors []float64, target float64) {
	for column := 0; column < regressionAccumulator.parameters; column++ {
		regressionAccumulator.xty[column] += predictors[column] * target

		for row := 0; row < regressionAccumulator.parameters; row++ {
			regressionAccumulator.xtx[row*regressionAccumulator.parameters+column] += predictors[row] * predictors[column]
		}
	}

	regressionAccumulator.yty += target * target
	regressionAccumulator.rows++
}

/*
prequentialPredict returns the model's prediction for one design row using the
model fit on every previously incorporated row, without allocating.
*/
func (regressionAccumulator *RegressionAccumulator) prequentialPredict(predictors []float64) (float64, bool) {
	if !regressionAccumulator.rlsReady {
		return 0, false
	}

	prediction := 0.0

	for column := 0; column < regressionAccumulator.parameters; column++ {
		prediction += regressionAccumulator.rlsW[column] * predictors[column]
	}

	return prediction, true
}

/*
prequentialAdd incorporates one design row after it has been scored by
prequentialPredict.
*/
func (regressionAccumulator *RegressionAccumulator) prequentialAdd(predictors []float64, target float64) {
	regressionAccumulator.add(predictors, target)

	if !regressionAccumulator.rlsReady {
		if regressionAccumulator.rows <= regressionAccumulator.parameters {
			return
		}

		if !regressionAccumulator.initializeRLS() {
			return
		}

		return
	}

	denominator := 1.0

	for row := 0; row < regressionAccumulator.parameters; row++ {
		sum := 0.0

		for column := 0; column < regressionAccumulator.parameters; column++ {
			sum += regressionAccumulator.rlsP[row*regressionAccumulator.parameters+column] * predictors[column]
		}

		regressionAccumulator.rlsScratch[row] = sum
		denominator += predictors[row] * sum
	}

	if denominator == 0 || math.IsNaN(denominator) {
		regressionAccumulator.rlsReady = false

		return
	}

	invDenominator := 1 / denominator
	errorTerm := target

	for column := 0; column < regressionAccumulator.parameters; column++ {
		errorTerm -= regressionAccumulator.rlsW[column] * predictors[column]
	}

	for row := 0; row < regressionAccumulator.parameters; row++ {
		gain := regressionAccumulator.rlsScratch[row] * invDenominator
		regressionAccumulator.rlsW[row] += gain * errorTerm

		for column := 0; column < regressionAccumulator.parameters; column++ {
			regressionAccumulator.rlsP[row*regressionAccumulator.parameters+column] -= gain * regressionAccumulator.rlsScratch[column]
		}
	}
}

/*
initializeRLS seeds the recursive least-squares state from the exact normal
equations at the first non-singular design.
*/
func (regressionAccumulator *RegressionAccumulator) initializeRLS() bool {
	if !invertLU(
		regressionAccumulator.xtx, regressionAccumulator.rlsP, regressionAccumulator.parameters,
		regressionAccumulator.luScratch, regressionAccumulator.pvtScratch, regressionAccumulator.colScratch,
	) {
		regressionAccumulator.rlsReady = false

		return false
	}

	if !solveLU(
		regressionAccumulator.xtx, regressionAccumulator.xty, regressionAccumulator.rlsW, regressionAccumulator.parameters,
		regressionAccumulator.luScratch, regressionAccumulator.pvtScratch,
	) {
		regressionAccumulator.rlsReady = false

		return false
	}

	regressionAccumulator.rlsReady = true

	return true
}

/*
fit solves the normal equations over the incorporated rows.
*/
func (regressionAccumulator *RegressionAccumulator) fit() RegressionFit {
	fit := RegressionFit{
		Observations:     regressionAccumulator.rows,
		Parameters:       regressionAccumulator.parameters,
		ResidualVariance: math.NaN(),
		Defined:          false,
	}

	if regressionAccumulator.rows <= regressionAccumulator.parameters {
		return fit
	}

	if !solveLU(
		regressionAccumulator.xtx, regressionAccumulator.xty, regressionAccumulator.fitCoefficients, regressionAccumulator.parameters,
		regressionAccumulator.luScratch, regressionAccumulator.pvtScratch,
	) {
		return fit
	}

	fit.Coefficients = append(fit.Coefficients[:0], regressionAccumulator.fitCoefficients...)

	sse := regressionAccumulator.yty

	for column := 0; column < regressionAccumulator.parameters; column++ {
		sse -= fit.Coefficients[column] * regressionAccumulator.xty[column]
	}

	if sse < 0 {
		sse = 0
	}

	fit.ResidualSSE = sse
	fit.ResidualVariance = sse / float64(regressionAccumulator.rows-regressionAccumulator.parameters)
	fit.CoefficientVariance = regressionAccumulator.coefficientVarianceFromCrossProduct(fit.ResidualVariance)
	fit.Defined = true

	return fit
}

/*
coefficientVarianceFromCrossProduct scales the diagonal of (X'X)⁻¹ by the
residual variance, reporting nil when the covariance is not identifiable.
*/
func (regressionAccumulator *RegressionAccumulator) coefficientVarianceFromCrossProduct(residualVariance float64) []float64 {
	if math.IsNaN(residualVariance) || residualVariance < 0 {
		return nil
	}

	if !invertLU(
		regressionAccumulator.xtx, regressionAccumulator.fitInverse, regressionAccumulator.parameters,
		regressionAccumulator.luScratch, regressionAccumulator.pvtScratch, regressionAccumulator.colScratch,
	) {
		return nil
	}

	variances := make([]float64, regressionAccumulator.parameters)

	for index := 0; index < regressionAccumulator.parameters; index++ {
		diag := regressionAccumulator.fitInverse[index*regressionAccumulator.parameters+index]
		variance := residualVariance * diag

		if variance < 0 || math.IsNaN(variance) {
			return nil
		}

		variances[index] = variance
	}

	return variances
}
