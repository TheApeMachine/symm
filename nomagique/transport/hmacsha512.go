package transport

import (
	"context"
)

type HMACSHA512Server struct {
	Downstream func(context.Context, any) error
}

func NewHMACSHA512() *HMACSHA512Server {
	return &HMACSHA512Server{}
}

func (s *HMACSHA512Server) Write(ctx context.Context, call HMACSHA512_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *HMACSHA512Server) Done(ctx context.Context, call HMACSHA512_done) error {
	return nil
}
