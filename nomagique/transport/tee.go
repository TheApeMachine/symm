package transport

import (
	"context"
)

type TeeServer struct {
	Downstream func(context.Context, any) error
}

func NewTeeServer() *TeeServer {
	return &TeeServer{}
}

func (s *TeeServer) Write(ctx context.Context, call Tee_write) error {
	if s.Downstream != nil {
		// Placeholder for Tee processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *TeeServer) Done(ctx context.Context, call Tee_done) error {
	return nil
}
