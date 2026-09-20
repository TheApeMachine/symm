package transport

import (
	"context"
)

type GateServer struct {
	Downstream func(context.Context, any) error
}

func NewGate() *GateServer {
	return &GateServer{}
}

func (s *GateServer) Write(ctx context.Context, call Gate_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *GateServer) Done(ctx context.Context, call Gate_done) error {
	return nil
}
