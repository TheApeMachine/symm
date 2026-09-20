package transport

import (
	"context"
)

type WSWriteServer struct {
	Downstream func(context.Context, any) error
}

func NewWSWriteServer() *WSWriteServer {
	return &WSWriteServer{}
}

func (s *WSWriteServer) Write(ctx context.Context, call WSWrite_write) error {
	if s.Downstream != nil {
		// Placeholder for WSWrite processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSWriteServer) Done(ctx context.Context, call WSWrite_done) error {
	return nil
}
