package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type PaceServer struct {
	Downstream func(context.Context, any) error
}

func NewPaceServer() *PaceServer {
	return &PaceServer{}
}

func (s *PaceServer) Write(ctx context.Context, call Pace_write) error {
	if s.Downstream != nil {
		// Placeholder for Pace processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *PaceServer) Done(ctx context.Context, call Pace_done) error {
	return nil
}



type PaceNode types.StreamNode[any, any]

func NewPace() PaceNode {
	server := &PaceServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
