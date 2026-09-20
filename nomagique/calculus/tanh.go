package calculus

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type TanhServer struct {
	Downstream types.Float64Sink
}

func (s *TanhServer) Write(ctx context.Context, call Tanh_write) error {
	a := call.Args().A()
	result := math.Tanh(a)
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *TanhServer) Done(ctx context.Context, call Tanh_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewTanh() *TanhServer {
	return &TanhServer{}
}
