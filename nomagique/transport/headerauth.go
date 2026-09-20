package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type HeaderAuthServer struct {
	Downstream func(context.Context, any) error
}

func NewHeaderAuthServer() *HeaderAuthServer {
	return &HeaderAuthServer{}
}

func (s *HeaderAuthServer) Write(ctx context.Context, call HeaderAuth_write) error {
	if s.Downstream != nil {
		// Placeholder for HeaderAuth processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *HeaderAuthServer) Done(ctx context.Context, call HeaderAuth_done) error {
	return nil
}



type HeaderAuthNode types.StreamNode[any, any]

func NewHeaderAuth() HeaderAuthNode {
	server := &HeaderAuthServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
