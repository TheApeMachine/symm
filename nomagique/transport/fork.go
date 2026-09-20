package transport

import (
	"context"
)

type ForkServer struct {
	Downstream func(context.Context, any) error
}

func NewForkServer() *ForkServer {
	return &ForkServer{}
}

func (s *ForkServer) Write(ctx context.Context, call Fork_write) error {
	if s.Downstream != nil {
		// Placeholder for Fork processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *ForkServer) Done(ctx context.Context, call Fork_done) error {
	return nil
}
