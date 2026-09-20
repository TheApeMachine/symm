package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type TeeServer struct {
	Downstream func(context.Context, any) error
}

func NewTeeServer() *TeeServer {
	return &TeeServer{}
}

func (s *TeeServer) Write(ctx context.Context, call Tee_write) error {
	if s.Downstream != nil {
		// Placeholder for Tee processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *TeeServer) Done(ctx context.Context, call Tee_done) error {
	return nil
}



type TeeNode types.StreamNode[any, any]

func NewTee() TeeNode {
	server := &TeeServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
