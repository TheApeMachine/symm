package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type BearerAuthServer struct {
	Downstream func(context.Context, any) error
}

func NewBearerAuthServer() *BearerAuthServer {
	return &BearerAuthServer{}
}

func (s *BearerAuthServer) Write(ctx context.Context, call BearerAuth_write) error {
	if s.Downstream != nil {
		// Placeholder for BearerAuth processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *BearerAuthServer) Done(ctx context.Context, call BearerAuth_done) error {
	return nil
}



type BearerAuthNode types.StreamNode[any, any]

func NewBearerAuth() BearerAuthNode {
	server := &BearerAuthServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
