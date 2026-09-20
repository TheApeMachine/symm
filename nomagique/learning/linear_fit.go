package learning

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

type LinearFitServer struct {
	DownstreamLinearFit func(context.Context, []float64) error
	Tolerance           float64
	Target              int
	Features            []int
}

func (s *LinearFitServer) Write(ctx context.Context, call LinearFit_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *LinearFitServer) WriteParams(ctx context.Context, callArgs LinearFit_write_Params) error {
	rowsList, _ := callArgs.Rows()
	rowCount := rowsList.Len()
	if rowCount == 0 {
		return nil
	}

	x := make([][]float64, 0, rowCount)
	y := make([][]float64, 0, rowCount)

	for r := 0; r < rowCount; r++ {
		rowItem := rowsList.At(r)
		rowListVals, _ := rowItem.Values()
		if s.Target < 0 || s.Target >= rowListVals.Len() {
			return nil
		}

		designRow := make([]float64, 1, len(s.Features)+1)
		designRow[0] = core.Unit

		for _, featureIdx := range s.Features {
			if featureIdx < 0 || featureIdx >= rowListVals.Len() {
				return nil
			}
			designRow = append(designRow, rowListVals.At(featureIdx))
		}
		x = append(x, designRow)
		y = append(y, []float64{rowListVals.At(s.Target)})
	}

	// OLS implementation inline to avoid circular deps
	cols := len(x[0])
	xtx := make([][]float64, cols)
	xty := make([][]float64, cols)
	for i := range xtx {
		xtx[i] = make([]float64, cols)
		xty[i] = make([]float64, 1)
	}
	for r := 0; r < len(x); r++ {
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
			return nil
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

	if s.DownstreamLinearFit != nil {
		return s.DownstreamLinearFit(ctx, coeffs)
	}
	return nil
}

func (s *LinearFitServer) Done(ctx context.Context, call LinearFit_done) error {
	return nil
}



type LinearFitNode types.StreamNode[any, any]

func NewLinearFit() LinearFitNode {
	server := &LinearFitServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
