package transport

import (
	"context"
)

type HMACSHA256Server struct {
	Downstream func(context.Context, any) error
}

func NewHMACSHA256() *HMACSHA256Server {
	return &HMACSHA256Server{}
}

func (s *HMACSHA256Server) Write(ctx context.Context, call HMACSHA256_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *HMACSHA256Server) Done(ctx context.Context, call HMACSHA256_done) error {
	return nil
}
