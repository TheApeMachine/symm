package calculus

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type SquareServer struct {
	Downstream types.Float64Sink
}

func (s *SquareServer) Write(ctx context.Context, call Square_write) error {
	a := call.Args().A()
	result := a * a
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *SquareServer) Done(ctx context.Context, call Square_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewSquare() *SquareServer {
	return &SquareServer{}
}
