package learning

import (
	"context"
	"math"
)

type DirectionalTargetServer struct {
	DownstreamDirectionalTarget func(context.Context, float64) error
	Deadband                    float64
}

func (s *DirectionalTargetServer) Write(ctx context.Context, call DirectionalTarget_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *DirectionalTargetServer) WriteParams(ctx context.Context, callArgs DirectionalTarget_write_Params) error {
	past := callArgs.Past()
	current := callArgs.Current()
	delta := current - past
	var result float64
	if math.Abs(delta) > s.Deadband {
		result = math.Copysign(1, delta)
	}
	if s.DownstreamDirectionalTarget != nil {
		return s.DownstreamDirectionalTarget(ctx, result)
	}
	return nil
}

func (s *DirectionalTargetServer) Done(ctx context.Context, call DirectionalTarget_done) error {
	return nil
}
