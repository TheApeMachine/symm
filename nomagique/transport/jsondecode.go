package transport

import (
	"context"
)

type JSONDecodeServer struct {
	Downstream func(context.Context, any) error
}

func NewJSONDecode() *JSONDecodeServer {
	return &JSONDecodeServer{}
}

func (s *JSONDecodeServer) Write(ctx context.Context, call JSONDecode_write) error {
	if s.Downstream != nil {
		return s.Downstream(ctx, nil)
	}

	return nil
}

func (s *JSONDecodeServer) Done(ctx context.Context, call JSONDecode_done) error {
	return nil
}
