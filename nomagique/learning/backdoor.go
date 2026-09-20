package learning

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

type BackdoorServer struct {
	DownstreamBackdoor func(context.Context, float64) error
	Tolerance          float64
	Features           []int
	Target             int
	Treatment          int
	Level              float64
	mean               float64
	count              float64
}

func (s *BackdoorServer) Write(ctx context.Context, call Backdoor_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *BackdoorServer) WriteParams(ctx context.Context, callArgs Backdoor_write_Params) error {
	historyList, _ := callArgs.History()

	// Fit OLS on history
	rowCount := historyList.Len()
	x := make([][]float64, 0, rowCount)
	y := make([][]float64, 0, rowCount)
	for r := 0; r < rowCount; r++ {
		rowItem := historyList.At(r)
		rowListVals, _ := rowItem.Values()
		designRow := make([]float64, 1, len(s.Features)+1)
		designRow[0] = core.Unit
		for _, featureIdx := range s.Features {
			designRow = append(designRow, rowListVals.At(featureIdx))
		}
		x = append(x, designRow)
		y = append(y, []float64{rowListVals.At(s.Target)})
	}
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

	expectation := 0.0
	s.count = 0
	s.mean = 0
	for r := 0; r < rowCount; r++ {
		rowItem := historyList.At(r)
		rowListVals, _ := rowItem.Values()
		intervened := make([]float64, rowListVals.Len())
		for i := 0; i < rowListVals.Len(); i++ {
			intervened[i] = rowListVals.At(i)
		}
		if s.Treatment < 0 || s.Treatment >= len(intervened) {
			return nil
		}
		intervened[s.Treatment] = s.Level

		designRow := make([]float64, 1, len(s.Features)+1)
		designRow[0] = core.Unit
		for _, feat := range s.Features {
			designRow = append(designRow, intervened[feat])
		}

		prediction := 0.0
		for i := 0; i < len(coeffs); i++ {
			prediction += coeffs[i] * designRow[i]
		}

		s.count++
		delta := prediction - s.mean
		s.mean += delta / s.count
		expectation = s.mean
	}

	if s.DownstreamBackdoor != nil {
		return s.DownstreamBackdoor(ctx, expectation)
	}
	return nil
}

func (s *BackdoorServer) Done(ctx context.Context, call Backdoor_done) error {
	return nil
}



type BackdoorNode types.StreamNode[any, any]

func NewBackdoor() BackdoorNode {
	server := &BackdoorServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
