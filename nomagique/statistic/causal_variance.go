package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type CausalVarianceServer struct {
	Downstream func(context.Context, float64) error
	count      float64
	mean       float64
	m2         float64
	prevVar    float64
}

func (s *CausalVarianceServer) Write(ctx context.Context, call CausalVariance_write) error {
	a := call.Args().A()
	ret := s.prevVar
	s.count++
	delta := a - s.mean
	s.mean += delta / s.count
	delta2 := a - s.mean
	s.m2 += delta * delta2
	if s.count > 1 {
		s.prevVar = s.m2 / (s.count - 1)
	}
	result := ret
	if s.count <= 2 {
		result = 0
	}

	return s.Downstream(ctx, result)
}

func (s *CausalVarianceServer) Done(ctx context.Context, call CausalVariance_done) error {
	return nil
}



type CausalVarianceNode types.StreamNode[any, any]

func NewCausalVariance() CausalVarianceNode {
	server := &CausalVarianceServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
