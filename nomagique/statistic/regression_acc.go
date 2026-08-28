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

	// fitCoefficients and fitInverse are Fit's scratch space, reused across
	// calls on the same accumulator. A prequential walk calls Fit once per
	// resident row (hundreds per candidate pair), so a fresh gonum vector and
	// matrix on every call was the single largest allocator in the process;
	// both are purely local to one Fit call and never retained past it.
	fitCoefficients []float64
	fitInverse      []float64

	// rlsP, rlsW, rlsScratch and rlsReady hold the recursive least-squares
	// prequential state: the running inverse (XᵀX)⁻¹ in row-major p×p form, the
	// running coefficient vector, a reusable p-vector scratch, and the
	// definiteness flag. They are populated lazily by PrequentialAdd and exist
	// so a prequential walk can score every row in O(p²) with zero allocations
	// instead of solving the normal equations (an LU factorization) per row.
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
model fit on every previously incorporated row, without allocating. It reports
false until the accumulated design is non-singular (more rows than parameters
and a well-defined inverse), mirroring Fit's Defined gate.

The running inverse (XᵀX)⁻¹ is maintained exactly, not via a regularized or
damped update: it is initialized once from the accumulated normal equations at
the first defined row, then updated with the textbook Sherman-Morrison rank-1
formula as each new row arrives, so every subsequent prediction is precisely the
ordinary-least-squares prequential one-step forecast, at O(p²) and zero
allocations per row.
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
PrequentialPredict. The incremental moments (xtx/xty/yty/rows) advance
identically to Add so the final exact Fit matches; the recursive inverse and
coefficient vector then advance with the rank-1 Sherman-Morrison update so the
next PrequentialPredict is allocation-free. The inverse is initialized from the
exact accumulated cross-product on the first row that leaves the design
non-singular; a singular update (denominator zero) leaves rlsReady false, which
is the same rank-deficiency signal Fit reports.
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

	// k = (P x) / (1 + xᵀ P x)
	denominator := 1.0

	for row := 0; row < accumulator.parameters; row++ {
		sum := 0.0

		for column := 0; column < accumulator.parameters; column++ {
			sum += accumulator.rlsP[row*accumulator.parameters+column] * predictors[column]
		}

		accumulator.rlsScratch[row] = sum
		denominator += predictors[row] * sum
	}

	if denominator == 0 {
		accumulator.rlsReady = false

		return
	}

	invDenominator := 1 / denominator
	errorTerm := target

	for column := 0; column < accumulator.parameters; column++ {
		errorTerm -= accumulator.rlsW[column] * predictors[column]
	}

	// w = w + (k * error); P = P - (k ⊗ (xᵀ P)) / denominator.
	for row := 0; row < accumulator.parameters; row++ {
		gain := accumulator.rlsScratch[row] * invDenominator
		accumulator.rlsW[row] += gain * errorTerm

		for column := 0; column < accumulator.parameters; column++ {
			accumulator.rlsP[row*accumulator.parameters+column] -= gain * accumulator.rlsScratch[column]
		}
	}
}

/*
initializeRLS seeds the recursive inverse and coefficient vector from the exact
accumulated normal equations at the first non-singular design. It returns false
when XᵀX is singular, matching Fit's rank-deficiency signal.
*/
func (accumulator *RegressionAccumulator) initializeRLS() bool {
	crossProduct := mat.NewDense(accumulator.parameters, accumulator.parameters, accumulator.xtx)
	inverse := mat.NewDense(accumulator.parameters, accumulator.parameters, accumulator.rlsP)

	if err := inverse.Inverse(crossProduct); err != nil {
		accumulator.rlsReady = false

		return false
	}

	rightHand := mat.NewVecDense(accumulator.parameters, accumulator.xty)
	coefficients := mat.NewVecDense(accumulator.parameters, accumulator.rlsW)

	if err := coefficients.SolveVec(crossProduct, rightHand); err != nil {
		accumulator.rlsReady = false

		return false
	}

	accumulator.rlsReady = true

	return true
}

/*
Fit solves the normal equations over the incorporated rows. It is undefined
when there are not more rows than parameters or when X'X is singular
(rank-deficient design).
*/
func (accumulator *RegressionAccumulator) Fit() RegressionFit {
	return accumulator.fit(true)
}

/*
Coefficients solves the normal equations for the coefficient vector only,
skipping the covariance inverse. It is the prequential-prediction shape of Fit:
a per-row walk needs the current coefficients to score the next observation but
never the coefficient variance, and the inverse is the single most expensive
per-call operation (a fresh Dense matrix and mat.Inverse). Defined, Predict, and
the residual scatter are identical to Fit; only CoefficientVariance is left
undefined (nil) and must be obtained from Fit at the end of the walk.
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

	crossProduct := mat.NewDense(accumulator.parameters, accumulator.parameters, accumulator.xtx)
	rightHand := mat.NewVecDense(accumulator.parameters, accumulator.xty)
	coefficients := mat.NewVecDense(accumulator.parameters, accumulator.fitCoefficients)

	if err := coefficients.SolveVec(crossProduct, rightHand); err != nil {
		// Singular cross-product: rank-deficient design.
		return fit
	}

	// SolveVec wrote the solution into the reused fitCoefficients backing
	// slice; copy it out directly without a fresh allocation or mat.Col scan.
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
		fit.CoefficientVariance = coefficientVarianceFromCrossProduct(
			crossProduct, accumulator.fitInverse, fit.ResidualVariance, accumulator.parameters,
		)
	}

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
scratch is the caller's reusable p×p backing slice for the inverse.
*/
func coefficientVarianceFromCrossProduct(crossProduct *mat.Dense, scratch []float64, residualVariance float64, parameters int) []float64 {
	inverse := mat.NewDense(parameters, parameters, scratch)

	if err := inverse.Inverse(crossProduct); err != nil {
		return nil
	}

	variance := make([]float64, parameters)

	for index := 0; index < parameters; index++ {
		variance[index] = residualVariance * inverse.At(index, index)
	}

	return variance
}
