package calculus

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type AbsoluteServer struct {
	Downstream types.Float64Sink
}

func (s *AbsoluteServer) Write(ctx context.Context, call Absolute_write) error {
	a := call.Args().A()
	result := math.Abs(a)
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *AbsoluteServer) Done(ctx context.Context, call Absolute_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewAbsolute() *AbsoluteServer {
	return &AbsoluteServer{}
}
