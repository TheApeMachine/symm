package transport

import (
	"context"
)

type WSBatchServer struct {
	Downstream func(context.Context, any) error
}

func NewWSBatchServer() *WSBatchServer {
	return &WSBatchServer{}
}

func (s *WSBatchServer) Write(ctx context.Context, call WSBatch_write) error {
	if s.Downstream != nil {
		// Placeholder for WSBatch processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSBatchServer) Done(ctx context.Context, call WSBatch_done) error {
	return nil
}
