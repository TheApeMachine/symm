package learning

import (
	"context"

	"github.com/theapemachine/symm/nomagique/core"
)

type LinearPredictionServer struct {
	DownstreamLinearPrediction func(context.Context, float64) error
	Features                   []int
}

func (s *LinearPredictionServer) Write(ctx context.Context, call LinearPrediction_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *LinearPredictionServer) WriteParams(ctx context.Context, callArgs LinearPrediction_write_Params) error {
	coeffs, _ := callArgs.Coefficients()
	raw, _ := callArgs.RawRow()

	if coeffs.Len() != len(s.Features)+1 {
		return nil // Coefficient length mismatch
	}

	designRow := make([]float64, 1, len(s.Features)+1)
	designRow[0] = core.Unit

	for _, feat := range s.Features {
		if feat < 0 || feat >= raw.Len() {
			return nil // Feature out of bounds
		}
		designRow = append(designRow, raw.At(feat))
	}

	sum := 0.0
	for i := 0; i < coeffs.Len(); i++ {
		sum += coeffs.At(i) * designRow[i]
	}

	if s.DownstreamLinearPrediction != nil {
		return s.DownstreamLinearPrediction(ctx, sum)
	}
	return nil
}

func (s *LinearPredictionServer) Done(ctx context.Context, call LinearPrediction_done) error {
	return nil
}

func NewLinearPrediction() *LinearPredictionServer {
	return &LinearPredictionServer{}
}
