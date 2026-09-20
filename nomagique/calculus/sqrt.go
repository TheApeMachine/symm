package calculus

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type SqrtServer struct {
	Downstream types.Float64Sink
}

func (s *SqrtServer) Write(ctx context.Context, call Sqrt_write) error {
	a := call.Args().A()
	result := math.Sqrt(a)
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *SqrtServer) Done(ctx context.Context, call Sqrt_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewSqrt() *SqrtServer {
	return &SqrtServer{}
}
