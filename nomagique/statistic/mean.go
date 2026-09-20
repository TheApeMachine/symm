package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type MeanServer struct {
	Downstream types.Float64Sink
	count      float64
	sum        float64
}

func (s *MeanServer) Write(ctx context.Context, call Mean_write) error {
	a := call.Args().A()
	s.count++
	s.sum += a
	result := s.sum / s.count
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *MeanServer) Done(ctx context.Context, call Mean_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewMean() *MeanServer {
	return &MeanServer{}
}
