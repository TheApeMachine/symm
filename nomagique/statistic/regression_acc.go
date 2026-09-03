package statistic

import (
	"math"
)

/*
RegressionAccumulator maintains the normal-equation moments (X'X, X'y, y'y)
of a linear regression incrementally across prequential steps, so each step's
fit costs O(p²) instead of refitting the full design from scratch. The
intercept is an explicit column of ones supplied by the caller.

Rank deficiency is explicit: the cross-product matrix is singular exactly
when the design is rank-deficient, and the fit is then undefined. No ridge,
dropped coordinate, or fabricated zero is substituted. Normal equations are
less numerically robust than a full SVD refit for ill-conditioned designs,
which is why rank is checked by the singularity of X'X; the mathematical
results match ordinary least squares for well-conditioned designs.
*/
type RegressionAccumulator struct {
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
}

/*
NewRegressionAccumulator builds an empty accumulator for a model with the
given parameter count (including the intercept column).
*/
func NewRegressionAccumulator(parameters int) *RegressionAccumulator {
	if parameters < 1 {
		parameters = 1
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
Parameters returns the model's parameter count.
*/
func (accumulator *RegressionAccumulator) Parameters() int {
	if accumulator == nil {
		return 0
	}

	return accumulator.parameters
}

/*
Rows returns the number of incorporated rows.
*/
func (accumulator *RegressionAccumulator) Rows() int {
	if accumulator == nil {
		return 0
	}

	return accumulator.rows
}

/*
Reset clears accumulated moments and RLS state so the accumulator's buffers
can be reused for a new candidate/lag without a fresh allocation.
*/
func (accumulator *RegressionAccumulator) Reset() {
	if accumulator == nil {
		return
	}

	clear(accumulator.xtx)
	clear(accumulator.xty)
	accumulator.yty = 0
	accumulator.rows = 0

	clear(accumulator.fitCoefficients)
	clear(accumulator.fitInverse)

	clear(accumulator.rlsP)
	clear(accumulator.rlsW)
	clear(accumulator.rlsScratch)
	accumulator.rlsReady = false
}

/*
Add incorporates one design row (length p, row-major with the intercept
first) and its target value.
*/
func (accumulator *RegressionAccumulator) Add(predictors []float64, target float64) {
	if accumulator == nil || len(predictors) != accumulator.parameters {
		return
	}

	for column := 0; column < accumulator.parameters; column++ {
		accumulator.xty[column] += predictors[column] * target

		for row := 0; row < accumulator.parameters; row++ {
			accumulator.xtx[row*accumulator.parameters+column] += predictors[row] * predictors[column]
		}
	}

	accumulator.yty += target * target
	accumulator.rows++
}

/*
PrequentialPredict returns the model's prediction for one design row using the
model fit on every previously incorporated row, without allocating.
*/
func (accumulator *RegressionAccumulator) PrequentialPredict(predictors []float64) (float64, bool) {
	if accumulator == nil || len(predictors) != accumulator.parameters || !accumulator.rlsReady {
		return 0, false
	}

	prediction := 0.0

	for column := 0; column < accumulator.parameters; column++ {
		prediction += accumulator.rlsW[column] * predictors[column]
	}

	return prediction, true
}

/*
PrequentialAdd incorporates one design row after it has been scored by
PrequentialPredict.
*/
func (accumulator *RegressionAccumulator) PrequentialAdd(predictors []float64, target float64) {
	if accumulator == nil || len(predictors) != accumulator.parameters {
		return
	}

	accumulator.Add(predictors, target)

	if !accumulator.rlsReady {
		if accumulator.rows <= accumulator.parameters {
			return
		}

		if !accumulator.initializeRLS() {
			return
		}
	}

	denominator := 1.0

	for row := 0; row < accumulator.parameters; row++ {
		sum := 0.0

		for column := 0; column < accumulator.parameters; column++ {
			sum += accumulator.rlsP[row*accumulator.parameters+column] * predictors[column]
		}

		accumulator.rlsScratch[row] = sum
		denominator += predictors[row] * sum
	}

	if denominator == 0 || math.IsNaN(denominator) {
		accumulator.rlsReady = false

		return
	}

	invDenominator := 1 / denominator
	errorTerm := target

	for column := 0; column < accumulator.parameters; column++ {
		errorTerm -= accumulator.rlsW[column] * predictors[column]
	}

	for row := 0; row < accumulator.parameters; row++ {
		gain := accumulator.rlsScratch[row] * invDenominator
		accumulator.rlsW[row] += gain * errorTerm

		for column := 0; column < accumulator.parameters; column++ {
			accumulator.rlsP[row*accumulator.parameters+column] -= gain * accumulator.rlsScratch[column]
		}
	}
}

func (accumulator *RegressionAccumulator) initializeRLS() bool {
	if !invertLU(
		accumulator.xtx, accumulator.rlsP, accumulator.parameters,
		accumulator.luScratch, accumulator.pvtScratch, accumulator.colScratch,
	) {
		accumulator.rlsReady = false

		return false
	}

	if !solveLU(
		accumulator.xtx, accumulator.xty, accumulator.rlsW, accumulator.parameters,
		accumulator.luScratch, accumulator.pvtScratch,
	) {
		accumulator.rlsReady = false

		return false
	}

	accumulator.rlsReady = true

	return true
}

/*
Fit solves the normal equations over the incorporated rows.
*/
func (accumulator *RegressionAccumulator) Fit() RegressionFit {
	return accumulator.fit(true)
}

/*
Coefficients solves the normal equations for the coefficient vector only.
*/
func (accumulator *RegressionAccumulator) Coefficients() RegressionFit {
	return accumulator.fit(false)
}

func (accumulator *RegressionAccumulator) fit(withVariance bool) RegressionFit {
	fit := RegressionFit{
		Observations:     accumulator.rows,
		Parameters:       accumulator.parameters,
		ResidualVariance: math.NaN(),
		Defined:          false,
	}

	if accumulator.rows <= accumulator.parameters {
		return fit
	}

	if !solveLU(
		accumulator.xtx, accumulator.xty, accumulator.fitCoefficients, accumulator.parameters,
		accumulator.luScratch, accumulator.pvtScratch,
	) {
		return fit
	}

	fit.Coefficients = append(fit.Coefficients[:0], accumulator.fitCoefficients...)

	sse := accumulator.yty

	for column := 0; column < accumulator.parameters; column++ {
		sse -= fit.Coefficients[column] * accumulator.xty[column]
	}

	if sse < 0 {
		sse = 0
	}

	fit.ResidualSSE = sse
	fit.ResidualVariance = sse / float64(accumulator.rows-accumulator.parameters)

	if withVariance {
		fit.CoefficientVariance = accumulator.coefficientVarianceFromCrossProduct(fit.ResidualVariance)
	}

	fit.Defined = true

	return fit
}

func (accumulator *RegressionAccumulator) coefficientVarianceFromCrossProduct(residualVariance float64) []float64 {
	if math.IsNaN(residualVariance) || residualVariance < 0 {
		return nil
	}

	if !invertLU(
		accumulator.xtx, accumulator.fitInverse, accumulator.parameters,
		accumulator.luScratch, accumulator.pvtScratch, accumulator.colScratch,
	) {
		return nil
	}

	variances := make([]float64, accumulator.parameters)

	for index := 0; index < accumulator.parameters; index++ {
		diag := accumulator.fitInverse[index*accumulator.parameters+index]
		variance := residualVariance * diag

		if variance < 0 || math.IsNaN(variance) {
			return nil
		}

		variances[index] = variance
	}

	return variances
}

/*
Predict evaluates the fitted model on one design row.
*/
func (fit RegressionFit) Predict(predictors []float64) (float64, bool) {
	if !fit.Defined || len(predictors) != fit.Parameters {
		return 0, false
	}

	predicted := 0.0

	for index, predictor := range predictors {
		predicted += fit.Coefficients[index] * predictor
	}

	return predicted, true
}

/*
VarianceAt returns the coefficient variance at a column index.
*/
func (fit RegressionFit) VarianceAt(column int) (float64, bool) {
	if column < 0 || column >= fit.Parameters || len(fit.CoefficientVariance) != fit.Parameters {
		return 0, false
	}

	return fit.CoefficientVariance[column], true
}

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
