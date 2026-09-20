package calculus

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type SignServer struct {
	Downstream types.Float64Sink
}

func (s *SignServer) Write(ctx context.Context, call Sign_write) error {
	a := call.Args().A()
	result := float64(0)
	if a < 0 {
		result = -1
	}

	if a > 0 {
		result = 1
	}

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *SignServer) Done(ctx context.Context, call Sign_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewSign() *SignServer {
	return &SignServer{}
}
