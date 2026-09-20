package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type CausalMeanServer struct {
	Downstream types.Float64Sink
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

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *CausalMeanServer) Done(ctx context.Context, call CausalMean_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewCausalMean() *CausalMeanServer {
	return &CausalMeanServer{}
}
