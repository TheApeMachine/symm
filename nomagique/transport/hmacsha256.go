package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type HMACSHA256Server struct {
	Downstream func(context.Context, any) error
}

func NewHMACSHA256Server() *HMACSHA256Server {
	return &HMACSHA256Server{}
}

func (s *HMACSHA256Server) Write(ctx context.Context, call HMACSHA256_write) error {
	if s.Downstream != nil {
		// Placeholder for HMACSHA256 processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *HMACSHA256Server) Done(ctx context.Context, call HMACSHA256_done) error {
	return nil
}



type HMACSHA256Node types.StreamNode[any, any]

func NewHMACSHA256() HMACSHA256Node {
	server := &HMACSHA256Server{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
