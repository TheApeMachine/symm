// Independent test oracle transcribed from the supplied statistic/lu.go.
package tests

import (
	"math"
)

/*
referenceSolveLU solves A x = b in-place using LU decomposition with partial pivoting.
a is p×p row-major and left untouched; lu and pvt are the caller's reusable
p×p and p-length scratch buffers. It returns false when A is singular to
working precision, matching the rank-deficiency signal callers already expect
from a normal-equations solve.
*/
func referenceSolveLU(a, b, x []float64, p int, lu []float64, pvt []int) bool {
	copy(lu, a)
	copy(x, b)

	for i := range p {
		pvt[i] = i
	}

	for k := range p {
		maxVal := math.Abs(lu[k*p+k])
		maxRow := k

		for i := k + 1; i < p; i++ {
			val := math.Abs(lu[i*p+k])
			if val > maxVal {
				maxVal = val
				maxRow = i
			}
		}

		if maxVal <= 1e-15 || math.IsNaN(maxVal) {
			return false
		}

		if maxRow != k {
			pvt[k], pvt[maxRow] = pvt[maxRow], pvt[k]

			for j := 0; j < p; j++ {
				lu[k*p+j], lu[maxRow*p+j] = lu[maxRow*p+j], lu[k*p+j]
			}

			x[k], x[maxRow] = x[maxRow], x[k]
		}

		pivotVal := lu[k*p+k]

		for i := k + 1; i < p; i++ {
			factor := lu[i*p+k] / pivotVal
			lu[i*p+k] = factor

			for j := k + 1; j < p; j++ {
				lu[i*p+j] -= factor * lu[k*p+j]
			}

			x[i] -= factor * x[k]
		}
	}

	for i := p - 1; i >= 0; i-- {
		sum := x[i]

		for j := i + 1; j < p; j++ {
			sum -= lu[i*p+j] * x[j]
		}

		x[i] = sum / lu[i*p+i]
	}

	return true
}

/*
referenceInvertLU computes inv = A⁻¹ in-place using LU decomposition with partial
pivoting. a is p×p row-major and left untouched; lu, pvt and col are the
caller's reusable p×p, p-length, and p-length scratch buffers.
*/
func referenceInvertLU(a, inv []float64, p int, lu []float64, pvt []int, col []float64) bool {
	copy(lu, a)

	for i := range p {
		pvt[i] = i
	}

	for k := range p {
		maxVal := math.Abs(lu[k*p+k])
		maxRow := k

		for i := k + 1; i < p; i++ {
			val := math.Abs(lu[i*p+k])
			if val > maxVal {
				maxVal = val
				maxRow = i
			}
		}

		if maxVal <= 1e-15 || math.IsNaN(maxVal) {
			return false
		}

		if maxRow != k {
			pvt[k], pvt[maxRow] = pvt[maxRow], pvt[k]

			for j := 0; j < p; j++ {
				lu[k*p+j], lu[maxRow*p+j] = lu[maxRow*p+j], lu[k*p+j]
			}
		}

		pivotVal := lu[k*p+k]

		for i := k + 1; i < p; i++ {
			factor := lu[i*p+k] / pivotVal
			lu[i*p+k] = factor

			for j := k + 1; j < p; j++ {
				lu[i*p+j] -= factor * lu[k*p+j]
			}
		}
	}

	for j := range p {
		for i := range p {
			col[i] = 0.0
		}

		for i := range p {
			if pvt[i] == j {
				col[i] = 1.0
				break
			}
		}

		for i := range p {
			sum := col[i]

			for k := 0; k < i; k++ {
				sum -= lu[i*p+k] * col[k]
			}

			col[i] = sum
		}

		for i := p - 1; i >= 0; i-- {
			sum := col[i]

			for k := i + 1; k < p; k++ {
				sum -= lu[i*p+k] * col[k]
			}

			col[i] = sum / lu[i*p+i]
		}

		for i := range p {
			inv[i*p+j] = col[i]
		}
	}

	return true
}
