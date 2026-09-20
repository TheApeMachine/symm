package transport

import (
	"context"
)

type TimestampServer struct {
	Downstream func(context.Context, any) error
}

func NewTimestampServer() *TimestampServer {
	return &TimestampServer{}
}

func (s *TimestampServer) Write(ctx context.Context, call Timestamp_write) error {
	if s.Downstream != nil {
		// Placeholder for Timestamp processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *TimestampServer) Done(ctx context.Context, call Timestamp_done) error {
	return nil
}
