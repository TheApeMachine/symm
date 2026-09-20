package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type NonceServer struct {
	Downstream func(context.Context, any) error
}

func NewNonceServer() *NonceServer {
	return &NonceServer{}
}

func (s *NonceServer) Write(ctx context.Context, call Nonce_write) error {
	if s.Downstream != nil {
		// Placeholder for Nonce processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *NonceServer) Done(ctx context.Context, call Nonce_done) error {
	return nil
}



type NonceNode types.StreamNode[any, any]

func NewNonce() NonceNode {
	server := &NonceServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
