package transport

import (
	"context"
)

type FanServer struct {
	Downstream func(context.Context, any) error
}

func NewFan() *FanServer {
	return &FanServer{}
}

func (s *FanServer) Write(ctx context.Context, call Fan_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *FanServer) Done(ctx context.Context, call Fan_done) error {
	return nil
}
