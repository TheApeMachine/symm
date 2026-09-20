package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type ResidualBaselineServer struct {
	Downstream types.Float64Sink
}

func (s *ResidualBaselineServer) Write(ctx context.Context, call ResidualBaseline_write) error {
	a := call.Args().A()
	result := a
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *ResidualBaselineServer) Done(ctx context.Context, call ResidualBaseline_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewResidualBaseline() *ResidualBaselineServer {
	return &ResidualBaselineServer{}
}
