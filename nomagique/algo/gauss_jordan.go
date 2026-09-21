package algo

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type GaussJordanServer struct {
	Tolerance float64
	x1        float64
	x2        float64
}

func (server *GaussJordanServer) Evaluate(ctx context.Context, left [][]float64, right [][]float64) ([][]float64, error) {
	rows := len(left)

	if rows == 0 {
		return nil, nil
	}

	tol := server.Tolerance

	if tol <= 0 {
		tol = 1e-9
	}

	rightCols := 0

	if rows > 0 && len(right) > 0 {
		rightCols = len(right[0])
	}

	matA := make([][]float64, rows)
	matB := make([][]float64, rows)

	for index := range rows {
		matA[index] = make([]float64, rows)
		copy(matA[index], left[index])

		matB[index] = make([]float64, rightCols)
		copy(matB[index], right[index])
	}

	for col := range rows {
		pivotRow := col
		maxVal := math.Abs(matA[col][col])

		for row := col + 1; row < rows; row++ {
			val := math.Abs(matA[row][col])

			if val > maxVal {
				maxVal = val
				pivotRow = row
			}
		}

		if maxVal <= tol {
			return nil, nil
		}

		if pivotRow != col {
			matA[col], matA[pivotRow] = matA[pivotRow], matA[col]
			matB[col], matB[pivotRow] = matB[pivotRow], matB[col]
		}

		pivot := matA[col][col]

		for index := col; index < rows; index++ {
			matA[col][index] /= pivot
		}

		for index := 0; index < rightCols; index++ {
			matB[col][index] /= pivot
		}

		for row := range rows {
			if row != col {
				factor := matA[row][col]

				for index := col; index < rows; index++ {
					matA[row][index] -= factor * matA[col][index]
				}

				for index := 0; index < rightCols; index++ {
					matB[row][index] -= factor * matB[col][index]
				}
			}
		}
	}

	return matB, nil
}

func (server *GaussJordanServer) Write(ctx context.Context, call GaussJordan_write) error {
	args := call.Args()
	a11 := args.A11()
	a12 := args.A12()
	a21 := args.A21()
	a22 := args.A22()
	valB1 := args.B1()
	valB2 := args.B2()

	det := a11*a22 - a12*a21
	tol := server.Tolerance

	if tol <= 0 {
		tol = server.Tolerance
	}

	if math.Abs(det) > tol {
		server.x1 = (valB1*a22 - a12*valB2) / det
		server.x2 = (a11*valB2 - valB1*a21) / det
	}

	return nil
}

func (server *GaussJordanServer) Done(ctx context.Context, call GaussJordan_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"gauss_jordan: alloc results failed",
			err,
		))
	}

	results.SetX1(server.x1)
	results.SetX2(server.x2)
	server.x1 = 0
	server.x2 = 0

	return nil
}

func NewGaussJordan() *GaussJordanServer {
	return &GaussJordanServer{Tolerance: 1e-9}
}
