package transport

import (
	"context"
)

type JSONEncodeServer struct {
	Downstream func(context.Context, any) error
}

func NewJSONEncodeServer() *JSONEncodeServer {
	return &JSONEncodeServer{}
}

func (s *JSONEncodeServer) Write(ctx context.Context, call JSONEncode_write) error {
	if s.Downstream != nil {
		// Placeholder for JSONEncode processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *JSONEncodeServer) Done(ctx context.Context, call JSONEncode_done) error {
	return nil
}
