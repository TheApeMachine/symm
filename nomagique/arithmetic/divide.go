package arithmetic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type DivideServer struct {
	Downstream types.Float64Sink
}

func (s *DivideServer) Write(ctx context.Context, call Divide_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := a / b
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *DivideServer) Done(ctx context.Context, call Divide_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewDivide() *DivideServer {
	return &DivideServer{}
}
