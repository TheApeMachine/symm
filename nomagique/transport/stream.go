package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type StreamServer struct {
	Downstream func(context.Context, any) error
}

func NewStreamServer() *StreamServer {
	return &StreamServer{}
}

func (s *StreamServer) Write(ctx context.Context, call Stream_write) error {
	if s.Downstream != nil {
		// Placeholder for Stream processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *StreamServer) Done(ctx context.Context, call Stream_done) error {
	return nil
}



type StreamNode types.StreamNode[any, any]

func NewStream() StreamNode {
	server := &StreamServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
