package transport

import (
	"context"
)

type PaceServer struct {
	Downstream func(context.Context, any) error
}

func NewPace() *PaceServer {
	return &PaceServer{}
}

func (s *PaceServer) Write(ctx context.Context, call Pace_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *PaceServer) Done(ctx context.Context, call Pace_done) error {
	return nil
}
