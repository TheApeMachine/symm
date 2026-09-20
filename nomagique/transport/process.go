package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type ProcessServer struct {
	Downstream func(context.Context, any) error
}

func NewProcessServer() *ProcessServer {
	return &ProcessServer{}
}

func (s *ProcessServer) Write(ctx context.Context, call Process_write) error {
	if s.Downstream != nil {
		// Placeholder for Process processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *ProcessServer) Done(ctx context.Context, call Process_done) error {
	return nil
}



type ProcessNode types.StreamNode[any, any]

func NewProcess() ProcessNode {
	server := &ProcessServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
