package learning

import (
	"github.com/theapemachine/symm/nomagique/types"
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



type IdentityTargetNode types.StreamNode[any, any]

func NewIdentityTarget() IdentityTargetNode {
	server := &IdentityTargetServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
