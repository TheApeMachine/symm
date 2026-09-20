package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type JSONDecodeServer struct {
	Downstream func(context.Context, any) error
}

func NewJSONDecodeServer() *JSONDecodeServer {
	return &JSONDecodeServer{}
}

func (s *JSONDecodeServer) Write(ctx context.Context, call JSONDecode_write) error {
	if s.Downstream != nil {
		// Placeholder for JSONDecode processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *JSONDecodeServer) Done(ctx context.Context, call JSONDecode_done) error {
	return nil
}



type JSONDecodeNode types.StreamNode[any, any]

func NewJSONDecode() JSONDecodeNode {
	server := &JSONDecodeServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
