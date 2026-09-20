package transport

import (
	"context"
)

type BroadcastServer struct {
	Downstream func(context.Context, any) error
}

func NewBroadcast() *BroadcastServer {
	return &BroadcastServer{}
}

func (s *BroadcastServer) Write(ctx context.Context, call Broadcast_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *BroadcastServer) Done(ctx context.Context, call Broadcast_done) error {
	return nil
}
