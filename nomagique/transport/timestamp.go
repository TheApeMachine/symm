package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
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



type TimestampNode types.StreamNode[any, any]

func NewTimestamp() TimestampNode {
	server := &TimestampServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
