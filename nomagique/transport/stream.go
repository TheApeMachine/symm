package transport

import (
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
