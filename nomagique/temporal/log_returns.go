package temporal

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type LogReturnsServer struct {
	Downstream  types.Float64Sink
	previous    float64
	initialized bool
}

func (s *LogReturnsServer) Write(ctx context.Context, call LogReturns_write) error {
	a := call.Args().A()
	if !s.initialized || s.previous <= 0 || a <= 0 {
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

	result := math.Log(a / s.previous)
	s.previous = a
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *LogReturnsServer) Done(ctx context.Context, call LogReturns_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewLogReturns() *LogReturnsServer {
	return &LogReturnsServer{}
}
