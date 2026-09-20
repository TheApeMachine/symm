package calculus

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type NegateServer struct {
	Downstream types.Float64Sink
}

func (s *NegateServer) Write(ctx context.Context, call Negate_write) error {
	a := call.Args().A()
	result := -a
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *NegateServer) Done(ctx context.Context, call Negate_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewNegate() *NegateServer {
	return &NegateServer{}
}
