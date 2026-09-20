package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type SquareServer struct {
	Downstream func(context.Context, float64) error
}

func (s *SquareServer) Write(ctx context.Context, call Square_write) error {
	a := call.Args().A()
	result := a * a

	return s.Downstream(ctx, result)
}

func (s *SquareServer) Done(ctx context.Context, call Square_done) error {
	return nil
}



type SquareNode types.StreamNode[any, any]

func NewSquare() SquareNode {
	server := &SquareServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
