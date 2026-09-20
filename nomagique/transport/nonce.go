package transport

import (
	"context"
)

type NonceServer struct {
	Downstream func(context.Context, any) error
}

func NewNonce() *NonceServer {
	return &NonceServer{}
}

func (s *NonceServer) Write(ctx context.Context, call Nonce_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *NonceServer) Done(ctx context.Context, call Nonce_done) error {
	return nil
}
