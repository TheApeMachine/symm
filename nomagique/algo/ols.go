package algo

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type OLSServer struct {
	Tolerance float64
	sumX      float64
	sumY      float64
	sumXX     float64
	sumXY     float64
	count     float64
	out       float64
}

func (server *OLSServer) Evaluate(ctx context.Context, x [][]float64, y [][]float64) ([]float64, error) {
	rows := len(x)
	
	if rows == 0 {
		return nil, nil
	}

	cols := len(x[0])

	xtx := make([][]float64, cols)
	xty := make([][]float64, cols)
	
	for index := range xtx {
		xtx[index] = make([]float64, cols)
		xty[index] = make([]float64, 1)
	}

	for row := range rows {
		for col1 := range cols {
			xVal := x[row][col1]

			for col2 := range cols {
				xtx[col1][col2] += xVal * x[row][col2]
			}
			
			xty[col1][0] += xVal * y[row][0]
		}
	}

	tol := server.Tolerance
	
	if tol <= 0 {
		tol = 1e-9
	}

	for col := 0; col < cols; col++ {
		pivotRow := col
		maxVal := math.Abs(xtx[col][col])
	
		for row := col + 1; row < cols; row++ {
			val := math.Abs(xtx[row][col])
			if val > maxVal {
				maxVal = val
				pivotRow = row
			}
		}

		if maxVal <= tol {
			return nil, nil
		}

		if pivotRow != col {
			xtx[col], xtx[pivotRow] = xtx[pivotRow], xtx[col]
			xty[col], xty[pivotRow] = xty[pivotRow], xty[col]
		}

		pivot := xtx[col][col]

		for index := col; index < cols; index++ {
			xtx[col][index] /= pivot
		}

		xty[col][0] /= pivot

		for row := 0; row < cols; row++ {
			if row != col {
				factor := xtx[row][col]
		
				for index := col; index < cols; index++ {
					xtx[row][index] -= factor * xtx[col][index]
				}
		
				xty[row][0] -= factor * xty[col][0]
			}
		}
	}

	coeffs := make([]float64, cols)
	
	for index := 0; index < cols; index++ {
		coeffs[index] = xty[index][0]
	}

	return coeffs, nil
}

func (server *OLSServer) Write(ctx context.Context, call OLS_write) error {
	valX := call.Args().X()
	valY := call.Args().Y()

	server.count++
	server.sumX += valX
	server.sumY += valY
	server.sumXX += valX * valX
	server.sumXY += valX * valY

	denom := server.count*server.sumXX - server.sumX*server.sumX
	if denom != 0 {
		slope := (server.count*server.sumXY - server.sumX*server.sumY) / denom
		server.out = slope
	}

	return nil
}

func (server *OLSServer) Done(ctx context.Context, call OLS_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"ols: alloc results failed",
			err,
		))
	}

	results.SetOut(server.out)
	server.out = 0
	return nil
}

func NewOLS() *OLSServer {
	return &OLSServer{Tolerance: 1e-9}
}
