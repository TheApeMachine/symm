package transport

import (
	"context"
)

type JoinServer struct {
	Downstream func(context.Context, any) error
}

func NewJoin() *JoinServer {
	return &JoinServer{}
}

func (s *JoinServer) Write(ctx context.Context, call Join_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *JoinServer) Done(ctx context.Context, call Join_done) error {
	return nil
}
