package transport

import (
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
