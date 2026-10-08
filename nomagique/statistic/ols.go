package statistic

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
OLSRequest is one ordinary least squares fit request: the row-major n×p design
matrix, the n targets, and the parameter count.
*/
type OLSRequest struct {
	X []float64
	Y []float64
	P int
}

/*
OLS owns one ordinary least squares fit as a Primitive. The caller owns the
design: an intercept column of ones is included only when the model requires
it. When rank < p the fit is undefined; no regularization is applied.
*/
type OLS struct {
	err error
	out OLSFit
}

/*
NewFitOLS instantiates the ordinary least squares Primitive.
*/
func NewFitOLS() core.Primitive {
	return &OLS{}
}

/*
Next fits every arriving request and hands over the resulting fit.
*/
func (op *OLS) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			request := (*OLSRequest)(arriving)
			op.out = fitOLS(request.X, request.Y, request.P)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *OLS) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
fitOLS fits y (length n) on the design matrix x (n×p, row-major). When
rank < p the fit is undefined; no regularization is applied.
*/
func fitOLS(x []float64, y []float64, p int) OLSFit {
	n := len(y)

	if p < 1 || n < 1 || len(x) < n*p {
		return OLSFit{Defined: false}
	}

	if n <= p {
		return OLSFit{
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

	if !solveLU(xtx, xty, coefficients, p, luScratch, pvtScratch) {
		return OLSFit{
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
	if invertLU(xtx, invScratch, p, luScratch, pvtScratch, colScratch) {
		variance = make([]float64, p)

		for index := 0; index < p; index++ {
			variance[index] = residualVariance * invScratch[index*p+index]
		}
	}

	return OLSFit{
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
CoefficientSNRPair carries one coefficient and the variance it must be judged
against.
*/
type CoefficientSNRPair struct {
	Coefficient float64
	Variance    float64
}

/*
CoefficientSNR owns the primary coefficient SNR, Coefficient² / Variance, as a
Primitive. It is non-negative and unbounded. It is not probability or
confidence, and it is undefined (NaN) when the coefficient variance is
unavailable or zero.
*/
type CoefficientSNR struct {
	err error
	out float64
}

/*
NewCoefficientSNR instantiates the coefficient signal-to-noise Primitive.
*/
func NewCoefficientSNR() core.Primitive {
	return &CoefficientSNR{}
}

/*
Next scores every arriving coefficient/variance pair and hands over the SNR.
*/
func (op *CoefficientSNR) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*CoefficientSNRPair)(arriving)
			op.out = coefficientSNR(pair.Coefficient, pair.Variance)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *CoefficientSNR) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
coefficientSNR returns Coefficient² / Variance, undefined (NaN) when the
coefficient variance is unavailable or zero.
*/
func coefficientSNR(coefficient float64, variance float64) float64 {
	if math.IsNaN(variance) || math.IsInf(variance, 0) || variance <= 0 {
		return math.NaN()
	}

	return coefficient * coefficient / variance
}
