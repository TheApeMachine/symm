// Independent test oracle transcribed from the supplied statistic/ols.go.
package tests

import (
	"math"
)

/*
referenceOLSFit is the result of one ordinary least squares fit. Rank deficiency is an
explicit state: when the design matrix does not have full column rank the fit
is Undefined, and no ridge, dropped coordinate, or fabricated zero is
substituted. Undefined is not zero.
*/
type referenceOLSFit struct {
	// Coefficients are the fitted parameters, one per design column in order.
	Coefficients []float64
	// CoefficientVariance is the diagonal of cov(beta) = sigma² (X'X)⁻¹.
	// It is nil when the covariance is not identifiable.
	CoefficientVariance []float64
	// Rank is the numerical rank of the design matrix.
	Rank int
	// Observations is the number of fitted rows.
	Observations int
	// Parameters is the number of fitted parameters (design columns).
	Parameters int
	// ResidualSSE is the sum of squared residuals.
	ResidualSSE float64
	// ResidualVariance is the unbiased residual variance SSE/(n-p) when
	// n > p, otherwise NaN.
	ResidualVariance float64
	// Defined reports whether the fit is mathematically identifiable.
	Defined bool
}

/*
referenceFitOLS fits y (length n) on the design matrix x (n×p, row-major). The caller
owns the design: an intercept column of ones is included only when the model
requires it. When rank < p the fit is undefined; no regularization is applied.
*/
func referenceFitOLS(x []float64, y []float64, p int) referenceOLSFit {
	n := len(y)

	if p < 1 || n < 1 || len(x) < n*p {
		return referenceOLSFit{Defined: false}
	}

	if n <= p {
		return referenceOLSFit{
			Observations:     n,
			Parameters:       p,
			ResidualVariance: math.NaN(),
			Defined:          false,
		}
	}

	xtx := make([]float64, p*p)
	xty := make([]float64, p)
	yty := 0.0

	for row := 0; row < n; row++ {
		rowOffset := row * p
		yVal := y[row]
		yty += yVal * yVal

		for column := 0; column < p; column++ {
			xty[column] += x[rowOffset+column] * yVal

			for k := 0; k <= column; k++ {
				xtx[column*p+k] += x[rowOffset+column] * x[rowOffset+k]
			}
		}
	}

	for column := 0; column < p; column++ {
		for k := 0; k < column; k++ {
			xtx[k*p+column] = xtx[column*p+k]
		}
	}

	coefficients := make([]float64, p)
	luScratch := make([]float64, p*p)
	pvtScratch := make([]int, p)

	if !referenceSolveLU(xtx, xty, coefficients, p, luScratch, pvtScratch) {
		return referenceOLSFit{
			Observations:     n,
			Parameters:       p,
			ResidualVariance: math.NaN(),
			Defined:          false,
		}
	}

	sse := yty

	for column := 0; column < p; column++ {
		sse -= coefficients[column] * xty[column]
	}

	if sse < 0 {
		sse = 0
	}

	residualVariance := sse / float64(n-p)

	invScratch := make([]float64, p*p)
	colScratch := make([]float64, p)

	var variance []float64
	if referenceInvertLU(xtx, invScratch, p, luScratch, pvtScratch, colScratch) {
		variance = make([]float64, p)

		for index := 0; index < p; index++ {
			variance[index] = residualVariance * invScratch[index*p+index]
		}
	}

	return referenceOLSFit{
		Coefficients:        coefficients,
		CoefficientVariance: variance,
		Rank:                p,
		Observations:        n,
		Parameters:          p,
		ResidualSSE:         sse,
		ResidualVariance:    residualVariance,
		Defined:             true,
	}
}

/*
VarianceAt returns the coefficient variance at a column index, reporting
false for negative, out-of-range, or unavailable entries so callers never
index an absent variance slice.
*/
func (fit referenceOLSFit) VarianceAt(column int) (float64, bool) {
	if column < 0 || column >= fit.Parameters || len(fit.CoefficientVariance) != fit.Parameters {
		return 0, false
	}

	return fit.CoefficientVariance[column], true
}

/*
referenceCoefficientSNR returns the primary coefficient SNR, Coefficient² / Variance.
It is non-negative and unbounded. It is not probability or confidence, and it
is undefined (NaN) when the coefficient variance is unavailable or zero.
*/
func referenceCoefficientSNR(coefficient float64, variance float64) float64 {
	if math.IsNaN(variance) || math.IsInf(variance, 0) || variance <= 0 {
		return math.NaN()
	}

	return coefficient * coefficient / variance
}
