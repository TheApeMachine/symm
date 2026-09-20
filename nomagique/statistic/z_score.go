package statistic

import (
	"context"
)

type ZScoreServer struct {
	Downstream func(context.Context, float64) error
	causalMean *CausalMeanServer
	causalVar  *CausalVarianceServer
}

func (s *ZScoreServer) Write(ctx context.Context, call ZScore_write) error {
	a := call.Args().A()
	// placeholder
	result := a

	return s.Downstream(ctx, result)
}

func (s *ZScoreServer) Done(ctx context.Context, call ZScore_done) error {
	return nil
}
