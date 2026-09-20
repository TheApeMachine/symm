package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type GateServer struct {
	Downstream func(context.Context, any) error
}

func NewGateServer() *GateServer {
	return &GateServer{}
}

func (s *GateServer) Write(ctx context.Context, call Gate_write) error {
	if s.Downstream != nil {
		// Placeholder for Gate processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *GateServer) Done(ctx context.Context, call Gate_done) error {
	return nil
}



type GateNode types.StreamNode[any, any]

func NewGate() GateNode {
	server := &GateServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
