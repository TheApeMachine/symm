package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
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



type Base64EncodeNode types.StreamNode[any, any]

func NewBase64Encode() Base64EncodeNode {
	server := &Base64EncodeServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
