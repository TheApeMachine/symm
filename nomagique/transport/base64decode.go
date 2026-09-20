package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type Base64DecodeServer struct {
	Downstream func(context.Context, any) error
}

func NewBase64DecodeServer() *Base64DecodeServer {
	return &Base64DecodeServer{}
}

func (s *Base64DecodeServer) Write(ctx context.Context, call Base64Decode_write) error {
	if s.Downstream != nil {
		// Placeholder for Base64Decode processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *Base64DecodeServer) Done(ctx context.Context, call Base64Decode_done) error {
	return nil
}



type Base64DecodeNode types.StreamNode[any, any]

func NewBase64Decode() Base64DecodeNode {
	server := &Base64DecodeServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
