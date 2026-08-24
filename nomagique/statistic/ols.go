package statistic

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

/*
OLSFit is the result of one ordinary least squares fit. Rank deficiency is an
explicit state: when the design matrix does not have full column rank the fit
is Undefined, and no ridge, dropped coordinate, or fabricated zero is
substituted. Undefined is not zero.
*/
type OLSFit struct {
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
machineEpsilon is the IEEE-754 double-precision unit roundoff used by the
numerical rank threshold. It is a model constant of floating point arithmetic,
not a tuning parameter.
*/
const machineEpsilon = 2.220446049250313e-16

/*
FitOLS fits y (length n) on the design matrix x (n×p, row-major). The caller
owns the design: an intercept column of ones is included only when the model
requires it. When rank < p the fit is undefined; no regularization is applied.
*/
func FitOLS(x []float64, y []float64, p int) OLSFit {
	n := len(y)

	if p < 1 || n < 1 || len(x) < n*p {
		return OLSFit{Defined: false}
	}

	if n <= p {
		return OLSFit{
			Observations:    n,
			Parameters:      p,
			ResidualVariance: math.NaN(),
			Defined:         false,
		}
	}

	xMatrix := mat.NewDense(n, p, x)
	yVector := mat.NewVecDense(n, y)

	var svd mat.SVD
	if !svd.Factorize(xMatrix, mat.SVDThin) {
		return OLSFit{
			Observations:     n,
			Parameters:       p,
			ResidualVariance: math.NaN(),
			Defined:          false,
		}
	}

	singularValues := svd.Values(nil)
	rank := numericalRank(singularValues, n, p)

	if rank < p {
		return OLSFit{
			Observations:     n,
			Parameters:       p,
			Rank:             rank,
			ResidualVariance: math.NaN(),
			Defined:          false,
		}
	}

	beta := mat.NewVecDense(p, nil)
	svd.SolveVecTo(beta, yVector, rank)

	coefficients := make([]float64, p)
	mat.Col(coefficients, 0, beta)

	sse := 0.0

	for row := 0; row < n; row++ {
		predicted := 0.0

		for column := 0; column < p; column++ {
			predicted += x[row*p+column] * coefficients[column]
		}

		residual := y[row] - predicted
		sse += residual * residual
	}

	residualVariance := sse / float64(n-p)
	variance := coefficientVariance(xMatrix, p, residualVariance)

	return OLSFit{
		Coefficients:        coefficients,
		CoefficientVariance: variance,
		Rank:                rank,
		Observations:        n,
		Parameters:          p,
		ResidualSSE:         sse,
		ResidualVariance:    residualVariance,
		Defined:             true,
	}
}

/*
numericalRank counts singular values above the standard LAPACK-style
threshold max(m,n) * eps * s_max. This is floating point rank determination,
not an evidence threshold.
*/
func numericalRank(singularValues []float64, rows int, columns int) int {
	if len(singularValues) == 0 {
		return 0
	}

	threshold := float64(max(rows, columns)) * machineEpsilon * singularValues[0]
	rank := 0

	for _, singularValue := range singularValues {
		if singularValue > threshold {
			rank++
		}
	}

	return rank
}

/*
coefficientVariance computes diag(sigma² (X'X)⁻¹). It returns nil when the
cross-product matrix cannot be inverted, which is an explicit undefined state
for coefficient uncertainty.
*/
func coefficientVariance(xMatrix *mat.Dense, p int, residualVariance float64) []float64 {
	crossProduct := mat.NewDense(p, p, nil)
	crossProduct.Product(xMatrix.T(), xMatrix)

	inverse := mat.NewDense(p, p, nil)

	if err := inverse.Inverse(crossProduct); err != nil {
		return nil
	}

	variance := make([]float64, p)

	for index := 0; index < p; index++ {
		variance[index] = residualVariance * inverse.At(index, index)
	}

	return variance
}

/*
CoefficientSNR returns the primary coefficient SNR, Coefficient² / Variance.
It is non-negative and unbounded. It is not probability or confidence, and it
is undefined (NaN) when the coefficient variance is unavailable or zero.
*/
func CoefficientSNR(coefficient float64, variance float64) float64 {
	if math.IsNaN(variance) || math.IsInf(variance, 0) || variance <= 0 {
		return math.NaN()
	}

	return coefficient * coefficient / variance
}
