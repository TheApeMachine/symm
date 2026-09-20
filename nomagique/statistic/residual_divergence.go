package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type ResidualDivergenceServer struct {
	Downstream func(context.Context, float64) error
	causalMean *CausalMeanServer
}

func (s *ResidualDivergenceServer) Write(ctx context.Context, call ResidualDivergence_write) error {
	a := call.Args().A()
	// placeholder
	result := a

	return s.Downstream(ctx, result)
}

func (s *ResidualDivergenceServer) Done(ctx context.Context, call ResidualDivergence_done) error {
	return nil
}



type ResidualDivergenceNode types.StreamNode[any, any]

func NewResidualDivergence() ResidualDivergenceNode {
	server := &ResidualDivergenceServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
