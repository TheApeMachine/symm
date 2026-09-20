package transport

import (
	"context"
)

type StreamServer struct {
	Downstream func(context.Context, any) error
}

func NewStream() *StreamServer {
	return &StreamServer{}
}

func (s *StreamServer) Write(ctx context.Context, call Stream_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *StreamServer) Done(ctx context.Context, call Stream_done) error {
	return nil
}
