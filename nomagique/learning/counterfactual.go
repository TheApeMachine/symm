package learning

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

type CounterfactualServer struct {
	DownstreamCounterfactual func(context.Context, float64, float64) error
	Tolerance                float64
	Target                   int
	Treatment                int
	Level                    float64
	Features                 []int
}

func (s *CounterfactualServer) Write(ctx context.Context, call Counterfactual_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *CounterfactualServer) WriteParams(ctx context.Context, callArgs Counterfactual_write_Params) error {
	historyList, _ := callArgs.History()
	factualRowList, _ := callArgs.FactualRow()

	// 1. Fit
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

	// 2. Factual residual
	factualOutcome := factualRowList.At(s.Target)
	designRow := make([]float64, 1, len(s.Features)+1)
	designRow[0] = core.Unit
	for _, feat := range s.Features {
		designRow = append(designRow, factualRowList.At(feat))
	}
	factualPrediction := 0.0
	for i := 0; i < len(coeffs); i++ {
		factualPrediction += coeffs[i] * designRow[i]
	}
	noise := factualOutcome - factualPrediction

	// 3. Intervention
	intervened := make([]float64, factualRowList.Len())
	for i := 0; i < factualRowList.Len(); i++ {
		intervened[i] = factualRowList.At(i)
	}
	if s.Treatment >= 0 && s.Treatment < len(intervened) {
		intervened[s.Treatment] = s.Level
	} else {
		return nil
	}

	// 4. Counterfactual Prediction
	cDesignRow := make([]float64, 1, len(s.Features)+1)
	cDesignRow[0] = core.Unit
	for _, feat := range s.Features {
		cDesignRow = append(cDesignRow, intervened[feat])
	}
	cPrediction := 0.0
	for i := 0; i < len(coeffs); i++ {
		cPrediction += coeffs[i] * cDesignRow[i]
	}
	cOutcome := cPrediction + noise

	// 5. Precision
	precision := core.Unit / (1.0 + math.Abs(noise))
	if s.DownstreamCounterfactual != nil {
		return s.DownstreamCounterfactual(ctx, cOutcome, precision)
	}
	return nil
}

func (s *CounterfactualServer) Done(ctx context.Context, call Counterfactual_done) error {
	return nil
}



type CounterfactualNode types.StreamNode[any, any]

func NewCounterfactual() CounterfactualNode {
	server := &CounterfactualServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
