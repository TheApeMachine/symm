package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type MaximumServer struct {
	Downstream func(context.Context, float64) error
}

func (s *MaximumServer) Write(ctx context.Context, call Maximum_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := math.Max(a, b)

	return s.Downstream(ctx, result)
}

func (s *MaximumServer) Done(ctx context.Context, call Maximum_done) error {
	return nil
}



type MaximumNode types.StreamNode[any, any]

func NewMaximum() MaximumNode {
	server := &MaximumServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
