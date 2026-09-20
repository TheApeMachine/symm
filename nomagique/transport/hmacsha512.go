package transport

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type HMACSHA512Server struct {
	Downstream func(context.Context, any) error
}

func NewHMACSHA512Server() *HMACSHA512Server {
	return &HMACSHA512Server{}
}

func (s *HMACSHA512Server) Write(ctx context.Context, call HMACSHA512_write) error {
	if s.Downstream != nil {
		// Placeholder for HMACSHA512 processing
		return s.Downstream(ctx, nil)
	}
	return nil
}

func (s *HMACSHA512Server) Done(ctx context.Context, call HMACSHA512_done) error {
	return nil
}



type HMACSHA512Node types.StreamNode[any, any]

func NewHMACSHA512() HMACSHA512Node {
	server := &HMACSHA512Server{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
