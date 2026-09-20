package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type WSConnectServer struct {
	Downstream func(context.Context, any) error
}

func NewWSConnectServer() *WSConnectServer {
	return &WSConnectServer{}
}

func (s *WSConnectServer) Write(ctx context.Context, call WSConnect_write) error {
	if s.Downstream != nil {
		// Placeholder for WSConnect processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSConnectServer) Done(ctx context.Context, call WSConnect_done) error {
	return nil
}



type WSConnectNode types.StreamNode[any, any]

func NewWSConnect() WSConnectNode {
	server := &WSConnectServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
