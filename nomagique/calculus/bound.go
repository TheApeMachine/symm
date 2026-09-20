package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type BoundServer struct {
	Downstream func(context.Context, float64) error
	Min        float64
	Max        float64
}

func (s *BoundServer) Write(ctx context.Context, call Bound_write) error {
	a := call.Args().A()
	result := math.Max(s.Min, math.Min(s.Max, a))
	return s.Downstream(ctx, result)
}

func (s *BoundServer) Done(ctx context.Context, call Bound_done) error {
	return nil
}



type BoundNode types.StreamNode[any, any]

func NewBound() BoundNode {
	server := &BoundServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
