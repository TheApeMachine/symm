package statistic

import (
	"math"

	"gonum.org/v1/gonum/mat"
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
		parameters: parameters,
		xtx:        make([]float64, parameters*parameters),
		xty:        make([]float64, parameters),
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
Fit solves the normal equations over the incorporated rows. It is undefined
when there are not more rows than parameters or when X'X is singular
(rank-deficient design).
*/
func (accumulator *RegressionAccumulator) Fit() RegressionFit {
	fit := RegressionFit{
		Observations:    accumulator.rows,
		Parameters:      accumulator.parameters,
		ResidualVariance: math.NaN(),
		Defined:         false,
	}

	if accumulator.rows <= accumulator.parameters {
		return fit
	}

	crossProduct := mat.NewDense(accumulator.parameters, accumulator.parameters, accumulator.xtx)
	rightHand := mat.NewVecDense(accumulator.parameters, accumulator.xty)
	coefficients := mat.NewVecDense(accumulator.parameters, nil)

	if err := coefficients.SolveVec(crossProduct, rightHand); err != nil {
		// Singular cross-product: rank-deficient design.
		return fit
	}

	fit.Coefficients = make([]float64, accumulator.parameters)
	mat.Col(fit.Coefficients, 0, coefficients)

	sse := accumulator.yty

	for column := 0; column < accumulator.parameters; column++ {
		sse -= fit.Coefficients[column] * accumulator.xty[column]
	}

	if sse < 0 {
		sse = 0
	}

	fit.ResidualSSE = sse
	fit.ResidualVariance = sse / float64(accumulator.rows-accumulator.parameters)
	fit.CoefficientVariance = coefficientVarianceFromCrossProduct(crossProduct, fit.ResidualVariance, accumulator.parameters)
	fit.Defined = true

	return fit
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
RegressionFit is the result of one accumulator fit. Undefined fields remain
undefined; rank deficiency is an explicit state.
*/
type RegressionFit struct {
	Coefficients        []float64
	CoefficientVariance []float64 // diag of sigma² (X'X)⁻¹; nil when unavailable
	ResidualSSE         float64
	ResidualVariance    float64
	Observations        int
	Parameters          int
	Defined             bool
}

/*
VarianceAt returns the coefficient variance at a column index, reporting
false for negative, out-of-range, or unavailable entries so callers never
index an absent variance slice.
*/
func (fit RegressionFit) VarianceAt(column int) (float64, bool) {
	if column < 0 || column >= fit.Parameters || len(fit.CoefficientVariance) != fit.Parameters {
		return 0, false
	}

	return fit.CoefficientVariance[column], true
}

/*
coefficientVarianceFromCrossProduct computes diag(sigma² (X'X)⁻¹) from the
already-formed cross-product matrix. It returns nil when the inverse is not
computable, which is an explicit undefined state for coefficient uncertainty.
*/
func coefficientVarianceFromCrossProduct(crossProduct *mat.Dense, residualVariance float64, parameters int) []float64 {
	inverse := mat.NewDense(parameters, parameters, nil)

	if err := inverse.Inverse(crossProduct); err != nil {
		return nil
	}

	variance := make([]float64, parameters)

	for index := 0; index < parameters; index++ {
		variance[index] = residualVariance * inverse.At(index, index)
	}

	return variance
}
