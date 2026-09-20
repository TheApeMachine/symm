package learning

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
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
	var result float64
	if past == 0 {
		result = math.NaN()
	} else {
		result = current/past - 1
	}
	if s.DownstreamRatioTarget != nil {
		return s.DownstreamRatioTarget(ctx, result)
	}
	return nil
}

func (s *RatioTargetServer) Done(ctx context.Context, call RatioTarget_done) error {
	return nil
}



type RatioTargetNode types.StreamNode[any, any]

func NewRatioTarget() RatioTargetNode {
	server := &RatioTargetServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
