package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type SqrtServer struct {
	Downstream func(context.Context, float64) error
}

func (s *SqrtServer) Write(ctx context.Context, call Sqrt_write) error {
	a := call.Args().A()
	result := math.Sqrt(a)

	return s.Downstream(ctx, result)
}

func (s *SqrtServer) Done(ctx context.Context, call Sqrt_done) error {
	return nil
}



type SqrtNode types.StreamNode[any, any]

func NewSqrt() SqrtNode {
	server := &SqrtServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
