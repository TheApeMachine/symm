package arithmetic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type MultiplyServer struct {
	Downstream types.Float64Sink
}

func (s *MultiplyServer) Write(ctx context.Context, call Multiply_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := a * b
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *MultiplyServer) Done(ctx context.Context, call Multiply_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewMultiply() *MultiplyServer {
	return &MultiplyServer{}
}
