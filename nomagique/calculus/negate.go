package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type NegateServer struct {
	Downstream func(context.Context, float64) error
}

func (s *NegateServer) Write(ctx context.Context, call Negate_write) error {
	a := call.Args().A()
	result := -a

	return s.Downstream(ctx, result)
}

func (s *NegateServer) Done(ctx context.Context, call Negate_done) error {
	return nil
}



type NegateNode types.StreamNode[any, any]

func NewNegate() NegateNode {
	server := &NegateServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
