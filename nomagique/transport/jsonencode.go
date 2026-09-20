package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
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



type JSONEncodeNode types.StreamNode[any, any]

func NewJSONEncode() JSONEncodeNode {
	server := &JSONEncodeServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
