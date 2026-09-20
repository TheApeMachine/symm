package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type RelativeChangeServer struct {
	Downstream  func(context.Context, float64) error
	previous    float64
	initialized bool
}

func (s *RelativeChangeServer) Write(ctx context.Context, call RelativeChange_write) error {
	a := call.Args().A()
	result := float64(0)
	if !s.initialized || s.previous == 0 {
		s.previous = a
		s.initialized = true
	} else {
		result = (a - s.previous) / s.previous
		s.previous = a
	}
	return s.Downstream(ctx, result)
}

func (s *RelativeChangeServer) Done(ctx context.Context, call RelativeChange_done) error {
	return nil
}



type RelativeChangeNode types.StreamNode[any, any]

func NewRelativeChange() RelativeChangeNode {
	server := &RelativeChangeServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
