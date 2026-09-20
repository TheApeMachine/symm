package temporal

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type ElapsedServer struct {
	Downstream  types.Float64Sink
	previous    int64
	initialized bool
}

func (s *ElapsedServer) Write(ctx context.Context, call Elapsed_write) error {
	a := call.Args().A()
	if !s.initialized {
		s.previous = a
		s.initialized = true
		if capnp.Client(s.Downstream).IsValid() {
			return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
				p.SetValue(0)
				return nil
			})
		}
		return nil
	}

	result := float64(a-s.previous) / 1e9
	s.previous = a
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *ElapsedServer) Done(ctx context.Context, call Elapsed_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewElapsed() *ElapsedServer {
	return &ElapsedServer{}
}
