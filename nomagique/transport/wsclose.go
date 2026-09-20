package transport

import (
	"context"
)

type WSCloseServer struct {
	Downstream func(context.Context, any) error
}

func NewWSCloseServer() *WSCloseServer {
	return &WSCloseServer{}
}

func (s *WSCloseServer) Write(ctx context.Context, call WSClose_write) error {
	if s.Downstream != nil {
		// Placeholder for WSClose processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSCloseServer) Done(ctx context.Context, call WSClose_done) error {
	return nil
}
