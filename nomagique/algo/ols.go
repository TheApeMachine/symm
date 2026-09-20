package algo

import (
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type OLSServer struct {
	DownstreamOLS func(context.Context, []float64) error
	Tolerance     float64
}

func (s *OLSServer) Evaluate(ctx context.Context, x [][]float64, y [][]float64) ([]float64, error) {
	rows := len(x)
	if rows == 0 {
		return nil, nil
	}
	cols := len(x[0])

	xtx := make([][]float64, cols)
	xty := make([][]float64, cols)
	for i := range xtx {
		xtx[i] = make([]float64, cols)
		xty[i] = make([]float64, 1)
	}
	for r := 0; r < rows; r++ {
		for i := 0; i < cols; i++ {
			xi := x[r][i]
			for j := 0; j < cols; j++ {
				xtx[i][j] += xi * x[r][j]
			}
			xty[i][0] += xi * y[r][0]
		}
	}
	tol := s.Tolerance
	if tol <= 0 {
		tol = 1e-9
	}
	for col := 0; col < cols; col++ {
		pivotRow := col
		maxVal := math.Abs(xtx[col][col])
		for r := col + 1; r < cols; r++ {
			val := math.Abs(xtx[r][col])
			if val > maxVal {
				maxVal = val
				pivotRow = r
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
		for c := col; c < cols; c++ {
			xtx[col][c] /= pivot
		}
		xty[col][0] /= pivot
		for r := 0; r < cols; r++ {
			if r != col {
				factor := xtx[r][col]
				for c := col; c < cols; c++ {
					xtx[r][c] -= factor * xtx[col][c]
				}
				xty[r][0] -= factor * xty[col][0]
			}
		}
	}
	coeffs := make([]float64, cols)
	for i := 0; i < cols; i++ {
		coeffs[i] = xty[i][0]
	}
	if s.DownstreamOLS != nil {
		return coeffs, s.DownstreamOLS(ctx, coeffs)
	}
	return coeffs, nil
}

func (s *OLSServer) Write(ctx context.Context, call OLS_write) error {
	xIn, _ := call.Args().X()
	yIn, _ := call.Args().Y()
	rows := xIn.Len()
	if rows == 0 {
		return nil
	}
	xIn0 := xIn.At(0)
	xIn0Vals, _ := xIn0.Values()
	cols := xIn0Vals.Len()

	x := make([][]float64, rows)
	y := make([][]float64, rows)
	for i := 0; i < rows; i++ {
		xInI := xIn.At(i)
		xInIVals, _ := xInI.Values()
		x[i] = make([]float64, cols)
		for j := 0; j < cols; j++ {
			x[i][j] = xInIVals.At(j)
		}

		yInI := yIn.At(i)
		yInIVals, _ := yInI.Values()
		y[i] = make([]float64, 1)
		y[i][0] = yInIVals.At(0)
	}
	
	_, err := s.Evaluate(ctx, x, y)
	return err
}

func (s *OLSServer) Done(ctx context.Context, call OLS_done) error {
	return nil
}

type OLSNode types.StreamNode[[2][][]float64, []float64]

func NewOLS(tolerance types.Float) OLSNode {
	server := &OLSServer{}
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		input := in.([2][][]float64)
		server.Tolerance = 1e-9
		if tolerance != nil {
			server.Tolerance = tolerance(input)
		}
		_, err := server.Evaluate(ctx, input[0], input[1])
		return err
	}, func(next func(context.Context, any) error) {
		server.DownstreamOLS = func(ctx context.Context, res []float64) error {
			return next(ctx, res)
		}
	})
}


