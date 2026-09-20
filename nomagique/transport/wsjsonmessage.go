package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type WSJSONMessageServer struct {
	Downstream func(context.Context, any) error
}

func NewWSJSONMessageServer() *WSJSONMessageServer {
	return &WSJSONMessageServer{}
}

func (s *WSJSONMessageServer) Write(ctx context.Context, call WSJSONMessage_write) error {
	if s.Downstream != nil {
		// Placeholder for WSJSONMessage processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSJSONMessageServer) Done(ctx context.Context, call WSJSONMessage_done) error {
	return nil
}



type WSJSONMessageNode types.StreamNode[any, any]

func NewWSJSONMessage() WSJSONMessageNode {
	server := &WSJSONMessageServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
