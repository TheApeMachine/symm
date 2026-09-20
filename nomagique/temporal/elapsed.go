package temporal

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type ElapsedServer struct {
	Downstream  func(context.Context, float64) error
	previous    int64
	initialized bool
}

func (s *ElapsedServer) Write(ctx context.Context, call Elapsed_write) error {
	a := call.Args().A()
	result := float64(0)
	if !s.initialized {
		s.previous = a
		s.initialized = true
	} else {
		result = float64(a-s.previous) / 1e9
		s.previous = a
	}
	return s.Downstream(ctx, result)
}

func (s *ElapsedServer) Done(ctx context.Context, call Elapsed_done) error {
	return nil
}



type ElapsedNode types.StreamNode[any, any]

func NewElapsed() ElapsedNode {
	server := &ElapsedServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
