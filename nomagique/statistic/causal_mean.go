package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type CausalMeanServer struct {
	Downstream func(context.Context, float64) error
	count      float64
	sum        float64
	prevMean   float64
}

func (s *CausalMeanServer) Write(ctx context.Context, call CausalMean_write) error {
	a := call.Args().A()
	ret := s.prevMean
	s.count++
	s.sum += a
	s.prevMean = s.sum / s.count
	result := ret
	if s.count == 1 {
		result = a
	}

	return s.Downstream(ctx, result)
}

func (s *CausalMeanServer) Done(ctx context.Context, call CausalMean_done) error {
	return nil
}



type CausalMeanNode types.StreamNode[any, any]

func NewCausalMean() CausalMeanNode {
	server := &CausalMeanServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
