package transport

import (
	"context"
)

type Base64EncodeServer struct {
	Downstream func(context.Context, any) error
}

func NewBase64EncodeServer() *Base64EncodeServer {
	return &Base64EncodeServer{}
}

func (s *Base64EncodeServer) Write(ctx context.Context, call Base64Encode_write) error {
	if s.Downstream != nil {
		// Placeholder for Base64Encode processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *Base64EncodeServer) Done(ctx context.Context, call Base64Encode_done) error {
	return nil
}
