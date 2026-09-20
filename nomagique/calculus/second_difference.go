package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type SecondDifferenceServer struct {
	Downstream func(context.Context, float64) error
	v1         float64
	v2         float64
	count      int
}

func (s *SecondDifferenceServer) Write(ctx context.Context, call SecondDifference_write) error {
	a := call.Args().A()
	s.count++
	d := a - s.v1
	d2 := d - s.v2
	s.v2 = d
	s.v1 = a
	result := float64(0)
	if s.count > 2 {
		result = d2
	}
	return s.Downstream(ctx, result)
}

func (s *SecondDifferenceServer) Done(ctx context.Context, call SecondDifference_done) error {
	return nil
}



type SecondDifferenceNode types.StreamNode[any, any]

func NewSecondDifference() SecondDifferenceNode {
	server := &SecondDifferenceServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
