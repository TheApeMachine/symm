package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type WSEncodeJSONServer struct {
	Downstream func(context.Context, any) error
}

func NewWSEncodeJSONServer() *WSEncodeJSONServer {
	return &WSEncodeJSONServer{}
}

func (s *WSEncodeJSONServer) Write(ctx context.Context, call WSEncodeJSON_write) error {
	if s.Downstream != nil {
		// Placeholder for WSEncodeJSON processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *WSEncodeJSONServer) Done(ctx context.Context, call WSEncodeJSON_done) error {
	return nil
}



type WSEncodeJSONNode types.StreamNode[any, any]

func NewWSEncodeJSON() WSEncodeJSONNode {
	server := &WSEncodeJSONServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
