package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type SubtractServer struct {
	Downstream func(context.Context, float64) error
}

func (s *SubtractServer) Write(ctx context.Context, call Subtract_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := a - b
	return s.Downstream(ctx, result)
}

func (s *SubtractServer) Done(ctx context.Context, call Subtract_done) error {
	return nil
}



type SubtractNode types.StreamNode[any, any]

func NewSubtract() SubtractNode {
	server := &SubtractServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
