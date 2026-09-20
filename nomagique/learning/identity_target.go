package learning

import (
	"context"
)

type IdentityTargetServer struct {
	DownstreamIdentityTarget func(context.Context, float64) error
}

func (s *IdentityTargetServer) Write(ctx context.Context, call IdentityTarget_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *IdentityTargetServer) WriteParams(ctx context.Context, callArgs IdentityTarget_write_Params) error {
	current := callArgs.Current()
	if s.DownstreamIdentityTarget != nil {
		return s.DownstreamIdentityTarget(ctx, current)
	}
	return nil
}

func (s *IdentityTargetServer) Done(ctx context.Context, call IdentityTarget_done) error {
	return nil
}
