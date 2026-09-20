package calculus

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type PolarizeServer struct {
	Downstream types.Float64Sink
}

func (s *PolarizeServer) Write(ctx context.Context, call Polarize_write) error {
	a := call.Args().A()
	b := call.Args().B()
	alpha := a
	if alpha < 0 {
		alpha = 0
	}

	beta := -a
	if beta < 0 {
		beta = 0
	}

	if b > 0 {
		alpha = alpha / (alpha + b)
		beta = beta / (beta + b)
	}

	result := alpha - beta
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *PolarizeServer) Done(ctx context.Context, call Polarize_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewPolarize() *PolarizeServer {
	return &PolarizeServer{}
}
