package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type SHA256Server struct {
	Downstream func(context.Context, any) error
}

func NewSHA256Server() *SHA256Server {
	return &SHA256Server{}
}

func (s *SHA256Server) Write(ctx context.Context, call SHA256_write) error {
	if s.Downstream != nil {
		// Placeholder for SHA256 processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *SHA256Server) Done(ctx context.Context, call SHA256_done) error {
	return nil
}



type SHA256Node types.StreamNode[any, any]

func NewSHA256() SHA256Node {
	server := &SHA256Server{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
