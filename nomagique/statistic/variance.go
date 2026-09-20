package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type VarianceServer struct {
	Downstream types.Float64Sink
	count      float64
	mean       float64
	m2         float64
}

func (s *VarianceServer) Write(ctx context.Context, call Variance_write) error {
	a := call.Args().A()
	s.count++
	delta := a - s.mean
	s.mean += delta / s.count
	delta2 := a - s.mean
	s.m2 += delta * delta2
	result := float64(0)
	if s.count > 1 {
		result = s.m2 / (s.count - 1)
	}

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *VarianceServer) Done(ctx context.Context, call Variance_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewVariance() *VarianceServer {
	return &VarianceServer{}
}
