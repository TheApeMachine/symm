package calculus

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type ReciprocalServer struct {
	Downstream types.Float64Sink
}

func (s *ReciprocalServer) Write(ctx context.Context, call Reciprocal_write) error {
	a := call.Args().A()
	result := float64(0)
	if a != 0 {
		result = 1.0 / a
	}

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *ReciprocalServer) Done(ctx context.Context, call Reciprocal_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewReciprocal() *ReciprocalServer {
	return &ReciprocalServer{}
}
