package transport

import (
	"context"
)

type DiscardServer struct {
	Downstream func(context.Context, any) error
}

func NewDiscardServer() *DiscardServer {
	return &DiscardServer{}
}

func (s *DiscardServer) Write(ctx context.Context, call Discard_write) error {
	if s.Downstream != nil {
		// Placeholder for Discard processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *DiscardServer) Done(ctx context.Context, call Discard_done) error {
	return nil
}
