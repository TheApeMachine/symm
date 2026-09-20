package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type MinimumServer struct {
	Downstream func(context.Context, float64) error
}

func (s *MinimumServer) Write(ctx context.Context, call Minimum_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := math.Min(a, b)

	return s.Downstream(ctx, result)
}

func (s *MinimumServer) Done(ctx context.Context, call Minimum_done) error {
	return nil
}



type MinimumNode types.StreamNode[any, any]

func NewMinimum() MinimumNode {
	server := &MinimumServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
