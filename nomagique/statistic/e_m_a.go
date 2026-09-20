package statistic

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type EMAServer struct {
	Downstream  types.Float64Sink
	ema         float64
	initialized bool
	Alpha       float64
}

func (s *EMAServer) Write(ctx context.Context, call EMA_write) error {
	a := call.Args().A()
	if s.Alpha <= 0 {
		if capnp.Client(s.Downstream).IsValid() {
			return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
				p.SetValue(a)
				return nil
			})
		}
		return nil
	}

	if !s.initialized {
		s.ema = a
		s.initialized = true
		if capnp.Client(s.Downstream).IsValid() {
			return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
				p.SetValue(s.ema)
				return nil
			})
		}
		return nil
	}

	s.ema = (a * s.Alpha) + (s.ema * (1.0 - s.Alpha))
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(s.ema)
			return nil
		})
	}
	return nil
}

func (s *EMAServer) Done(ctx context.Context, call EMA_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewEMA() *EMAServer {
	return &EMAServer{}
}
