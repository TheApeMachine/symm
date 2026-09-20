package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type CausalVarianceServer struct {
	Downstream types.Float64Sink
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

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *CausalVarianceServer) Done(ctx context.Context, call CausalVariance_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewCausalVariance() *CausalVarianceServer {
	return &CausalVarianceServer{}
}
