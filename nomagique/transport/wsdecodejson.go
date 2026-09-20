package transport

import (
	"context"
)

type WSDecodeJSONServer struct {
	Downstream func(context.Context, any) error
}

func NewWSDecodeJSON() *WSDecodeJSONServer {
	return &WSDecodeJSONServer{}
}

func (s *WSDecodeJSONServer) Write(ctx context.Context, call WSDecodeJSON_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *WSDecodeJSONServer) Done(ctx context.Context, call WSDecodeJSON_done) error {
	return nil
}
