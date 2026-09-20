package transport

import (
	"context"
)

type HeaderAuthServer struct {
	Downstream func(context.Context, any) error
}

func NewHeaderAuth() *HeaderAuthServer {
	return &HeaderAuthServer{}
}

func (s *HeaderAuthServer) Write(ctx context.Context, call HeaderAuth_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *HeaderAuthServer) Done(ctx context.Context, call HeaderAuth_done) error {
	return nil
}
