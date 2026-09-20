package learning

import (
	"context"
)

type RatioTargetServer struct {
	DownstreamRatioTarget func(context.Context, float64) error
}

func (s *RatioTargetServer) Write(ctx context.Context, call RatioTarget_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *RatioTargetServer) WriteParams(ctx context.Context, callArgs RatioTarget_write_Params) error {
	past := callArgs.Past()
	current := callArgs.Current()
	result := current/past - 1

	if s.DownstreamRatioTarget != nil {
		return s.DownstreamRatioTarget(ctx, result)
	}

	return nil
}

func (s *RatioTargetServer) Done(ctx context.Context, call RatioTarget_done) error {
	return nil
}

func NewRatioTarget() *RatioTargetServer {
	return &RatioTargetServer{}
}
