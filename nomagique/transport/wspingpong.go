package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type WSPingPongServer struct {
	Downstream func(context.Context, any) error
}

func NewWSPingPongServer() *WSPingPongServer {
	return &WSPingPongServer{}
}

func (s *WSPingPongServer) Write(ctx context.Context, call WSPingPong_write) error {
	if s.Downstream != nil {
		// Placeholder for WSPingPong processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSPingPongServer) Done(ctx context.Context, call WSPingPong_done) error {
	return nil
}



type WSPingPongNode types.StreamNode[any, any]

func NewWSPingPong() WSPingPongNode {
	server := &WSPingPongServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
