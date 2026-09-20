package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type WSReadServer struct {
	Downstream func(context.Context, any) error
}

func NewWSReadServer() *WSReadServer {
	return &WSReadServer{}
}

func (s *WSReadServer) Write(ctx context.Context, call WSRead_write) error {
	if s.Downstream != nil {
		// Placeholder for WSRead processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSReadServer) Done(ctx context.Context, call WSRead_done) error {
	return nil
}



type WSReadNode types.StreamNode[any, any]

func NewWSRead() WSReadNode {
	server := &WSReadServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
