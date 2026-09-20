package learning

import (
	"context"
)

type DeltaTargetServer struct {
	DownstreamDeltaTarget func(context.Context, float64) error
}

func (s *DeltaTargetServer) Write(ctx context.Context, call DeltaTarget_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *DeltaTargetServer) WriteParams(ctx context.Context, callArgs DeltaTarget_write_Params) error {
	past := callArgs.Past()
	current := callArgs.Current()
	result := current - past
	if s.DownstreamDeltaTarget != nil {
		return s.DownstreamDeltaTarget(ctx, result)
	}
	return nil
}

func (s *DeltaTargetServer) Done(ctx context.Context, call DeltaTarget_done) error {
	return nil
}
