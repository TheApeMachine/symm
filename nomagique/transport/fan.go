package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type FanServer struct {
	Downstream func(context.Context, any) error
}

func NewFanServer() *FanServer {
	return &FanServer{}
}

func (s *FanServer) Write(ctx context.Context, call Fan_write) error {
	if s.Downstream != nil {
		// Placeholder for Fan processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *FanServer) Done(ctx context.Context, call Fan_done) error {
	return nil
}



type FanNode types.StreamNode[any, any]

func NewFan() FanNode {
	server := &FanServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
