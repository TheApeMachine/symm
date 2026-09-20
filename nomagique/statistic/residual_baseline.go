package statistic

import (
	"context"
)

type ResidualBaselineServer struct {
	Downstream func(context.Context, float64) error
	causalMean *CausalMeanServer
}

func (s *ResidualBaselineServer) Write(ctx context.Context, call ResidualBaseline_write) error {
	a := call.Args().A()
	// placeholder for composition
	result := a

	return s.Downstream(ctx, result)
}

func (s *ResidualBaselineServer) Done(ctx context.Context, call ResidualBaseline_done) error {
	return nil
}
