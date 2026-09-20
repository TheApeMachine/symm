package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type WSDecodeJSONServer struct {
	Downstream func(context.Context, any) error
}

func NewWSDecodeJSONServer() *WSDecodeJSONServer {
	return &WSDecodeJSONServer{}
}

func (s *WSDecodeJSONServer) Write(ctx context.Context, call WSDecodeJSON_write) error {
	if s.Downstream != nil {
		// Placeholder for WSDecodeJSON processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSDecodeJSONServer) Done(ctx context.Context, call WSDecodeJSON_done) error {
	return nil
}



type WSDecodeJSONNode types.StreamNode[any, any]

func NewWSDecodeJSON() WSDecodeJSONNode {
	server := &WSDecodeJSONServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
