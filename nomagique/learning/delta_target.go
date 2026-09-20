package learning

import (
	"github.com/theapemachine/symm/nomagique/types"
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



type DeltaTargetNode types.StreamNode[any, any]

func NewDeltaTarget() DeltaTargetNode {
	server := &DeltaTargetServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
