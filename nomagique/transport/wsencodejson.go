package transport

import (
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
