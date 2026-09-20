package transport

import (
	"context"
)

type JoinServer struct {
	Downstream func(context.Context, any) error
}

func NewJoinServer() *JoinServer {
	return &JoinServer{}
}

func (s *JoinServer) Write(ctx context.Context, call Join_write) error {
	if s.Downstream != nil {
		// Placeholder for Join processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *JoinServer) Done(ctx context.Context, call Join_done) error {
	return nil
}
