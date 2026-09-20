package transport

import (
	"context"
)

type SHA256Server struct {
	Downstream func(context.Context, any) error
}

func NewSHA256() *SHA256Server {
	return &SHA256Server{}
}

func (s *SHA256Server) Write(ctx context.Context, call SHA256_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *SHA256Server) Done(ctx context.Context, call SHA256_done) error {
	return nil
}
