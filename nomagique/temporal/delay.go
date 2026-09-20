package temporal

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type DelayServer struct {
	Downstream types.Float64Sink
	horizon    int
	buffer     []float64
	idx        int
}

func (s *DelayServer) Write(ctx context.Context, call Delay_write) error {
	val := call.Args().A()
	if s.horizon <= 0 {
		s.horizon = 1
	}

	if len(s.buffer) < s.horizon {
		s.buffer = append(s.buffer, val)
		return nil
	}

	delayed := s.buffer[s.idx]
	s.buffer[s.idx] = val
	s.idx = (s.idx + 1) % s.horizon

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(delayed)
			return nil
		})
	}
	return nil
}

func (s *DelayServer) Done(ctx context.Context, call Delay_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewDelay() *DelayServer {
	return &DelayServer{horizon: 1}
}
