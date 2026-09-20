package temporal

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type VelocityServer struct {
	Downstream func(context.Context, float64) error
	prevValue  float64
	prevTime   float64
	hasPrior   bool
}

func (s *VelocityServer) Write(ctx context.Context, call Velocity_write) error {
	val := call.Args().Val()
	ts := call.Args().Ts()
	result := float64(0)
	if !s.hasPrior {
		s.prevValue = val
		s.prevTime = ts
		s.hasPrior = true
	} else {
		diff := val - s.prevValue
		elapsed := ts - s.prevTime
		s.prevValue = val
		s.prevTime = ts
		if elapsed > 0 {
			result = diff / elapsed
		}
	}
	return s.Downstream(ctx, result)
}

func (s *VelocityServer) Done(ctx context.Context, call Velocity_done) error {
	return nil
}



type VelocityNode types.StreamNode[any, any]

func NewVelocity() VelocityNode {
	server := &VelocityServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
