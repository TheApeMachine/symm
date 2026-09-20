package transport

import (
	"context"
)

type BearerAuthServer struct {
	Downstream func(context.Context, any) error
}

func NewBearerAuth() *BearerAuthServer {
	return &BearerAuthServer{}
}

func (s *BearerAuthServer) Write(ctx context.Context, call BearerAuth_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *BearerAuthServer) Done(ctx context.Context, call BearerAuth_done) error {
	return nil
}
