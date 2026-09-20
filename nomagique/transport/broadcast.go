package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type BroadcastServer struct {
	Downstream func(context.Context, any) error
}

func NewBroadcastServer() *BroadcastServer {
	return &BroadcastServer{}
}

func (s *BroadcastServer) Write(ctx context.Context, call Broadcast_write) error {
	if s.Downstream != nil {
		// Placeholder for Broadcast processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *BroadcastServer) Done(ctx context.Context, call Broadcast_done) error {
	return nil
}



type BroadcastNode types.StreamNode[any, any]

func NewBroadcast() BroadcastNode {
	server := &BroadcastServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
