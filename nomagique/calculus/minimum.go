package calculus

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type MinimumServer struct {
	Downstream types.Float64Sink
}

func (s *MinimumServer) Write(ctx context.Context, call Minimum_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := math.Min(a, b)
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *MinimumServer) Done(ctx context.Context, call Minimum_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewMinimum() *MinimumServer {
	return &MinimumServer{}
}
