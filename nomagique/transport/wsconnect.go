package transport

import (
	"context"
)

type WSConnectServer struct {
	Downstream func(context.Context, any) error
}

func NewWSConnect() *WSConnectServer {
	return &WSConnectServer{}
}

func (s *WSConnectServer) Write(ctx context.Context, call WSConnect_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *WSConnectServer) Done(ctx context.Context, call WSConnect_done) error {
	return nil
}
