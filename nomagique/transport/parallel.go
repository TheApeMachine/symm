package transport

import (
	"context"
)

type ParallelServer struct {
	Downstream func(context.Context, any) error
}

func NewParallelServer() *ParallelServer {
	return &ParallelServer{}
}

func (s *ParallelServer) Write(ctx context.Context, call Parallel_write) error {
	if s.Downstream != nil {
		// Placeholder for Parallel processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *ParallelServer) Done(ctx context.Context, call Parallel_done) error {
	return nil
}
