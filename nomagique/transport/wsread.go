package transport

import (
	"context"
)

type WSReadServer struct {
	Downstream func(context.Context, any) error
}

func NewWSRead() *WSReadServer {
	return &WSReadServer{}
}

func (s *WSReadServer) Write(ctx context.Context, call WSRead_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *WSReadServer) Done(ctx context.Context, call WSRead_done) error {
	return nil
}
