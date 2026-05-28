package risk

import "gonum.org/v1/gonum/mat"

/*
Matrix is a symmetric correlation matrix for systemic concentration.
*/
type Matrix struct {
	rows [][]float64
}

/*
PrincipalEigenvalue returns the largest eigenvalue of the correlation matrix.
*/
func (matrix *Matrix) PrincipalEigenvalue() (float64, bool) {
	if matrix == nil || len(matrix.rows) == 0 {
		return 0, false
	}

	size := len(matrix.rows)
	data := make([]float64, size*size)

	for row := range matrix.rows {
		if len(matrix.rows[row]) != size {
			return 0, false
		}

		for col := range matrix.rows[row] {
			data[row*size+col] = matrix.rows[row][col]
		}
	}

	var eigen mat.EigenSym

	if !eigen.Factorize(mat.NewSymDense(size, data), false) {
		return 0, false
	}

	values := eigen.Values(nil)
	peak := values[0]

	for _, value := range values[1:] {
		if value > peak {
			peak = value
		}
	}

	return peak, true
}

/*
SystemicConcentration maps the principal eigenvalue to a unit concentration score.
*/
func (matrix *Matrix) SystemicConcentration() (float64, bool) {
	eigenvalue, ok := matrix.PrincipalEigenvalue()

	if !ok || len(matrix.rows) < 2 {
		return 0, false
	}

	concentration := (eigenvalue - 1) / (float64(len(matrix.rows)) - 1)

	return clampUnit(concentration), true
}
