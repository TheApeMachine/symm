package transport

import (
	"context"
)

type Base64DecodeServer struct {
	Downstream func(context.Context, any) error
}

func NewBase64Decode() *Base64DecodeServer {
	return &Base64DecodeServer{}
}

func (s *Base64DecodeServer) Write(ctx context.Context, call Base64Decode_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *Base64DecodeServer) Done(ctx context.Context, call Base64Decode_done) error {
	return nil
}
