package temporal

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type VelocityServer struct {
	Downstream types.Float64Sink
	prevValue  float64
	prevTime   float64
	hasPrior   bool
}

func (s *VelocityServer) Write(ctx context.Context, call Velocity_write) error {
	val := call.Args().Val()
	ts := call.Args().Ts()
	if !s.hasPrior {
		s.prevValue = val
		s.prevTime = ts
		s.hasPrior = true
		if capnp.Client(s.Downstream).IsValid() {
			return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
				p.SetValue(0)
				return nil
			})
		}
		return nil
	}

	diff := val - s.prevValue
	elapsed := ts - s.prevTime
	s.prevValue = val
	s.prevTime = ts

	result := float64(0)
	if elapsed > 0 {
		result = diff / elapsed
	}

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *VelocityServer) Done(ctx context.Context, call Velocity_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewVelocity() *VelocityServer {
	return &VelocityServer{}
}
