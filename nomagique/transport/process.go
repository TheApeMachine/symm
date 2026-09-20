package transport

import (
	"context"
)

type ProcessServer struct {
	Downstream func(context.Context, any) error
}

func NewProcess() *ProcessServer {
	return &ProcessServer{}
}

func (s *ProcessServer) Write(ctx context.Context, call Process_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *ProcessServer) Done(ctx context.Context, call Process_done) error {
	return nil
}
