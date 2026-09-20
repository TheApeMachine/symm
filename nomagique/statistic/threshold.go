package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type ThresholdServer struct {
	Downstream types.Float64Sink
	Band       float64
	Rest       float64
	Lower      float64
	Upper      float64
}

func (s *ThresholdServer) Write(ctx context.Context, call Threshold_write) error {
	a := call.Args().A()
	result := s.Rest
	if a < s.Band {
		result = s.Upper
	}

	if a > 1.0-s.Band {
		result = s.Lower
	}

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *ThresholdServer) Done(ctx context.Context, call Threshold_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewThreshold() *ThresholdServer {
	return &ThresholdServer{}
}
