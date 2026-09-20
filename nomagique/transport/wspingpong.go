package transport

import (
	"context"
)

type WSPingPongServer struct {
	Downstream func(context.Context, any) error
}

func NewWSPingPong() *WSPingPongServer {
	return &WSPingPongServer{}
}

func (s *WSPingPongServer) Write(ctx context.Context, call WSPingPong_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *WSPingPongServer) Done(ctx context.Context, call WSPingPong_done) error {
	return nil
}
