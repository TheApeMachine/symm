package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type AbsoluteServer struct {
	Downstream func(context.Context, float64) error
}

func (s *AbsoluteServer) Write(ctx context.Context, call Absolute_write) error {
	a := call.Args().A()
	result := math.Abs(a)

	return s.Downstream(ctx, result)
}

func (s *AbsoluteServer) Done(ctx context.Context, call Absolute_done) error {
	return nil
}



type AbsoluteNode types.StreamNode[any, any]

func NewAbsolute() AbsoluteNode {
	server := &AbsoluteServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
