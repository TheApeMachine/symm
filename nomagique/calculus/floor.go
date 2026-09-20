package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type FloorServer struct {
	Downstream func(context.Context, float64) error
}

func (s *FloorServer) Write(ctx context.Context, call Floor_write) error {
	a := call.Args().A()
	result := math.Floor(a)

	return s.Downstream(ctx, result)
}

func (s *FloorServer) Done(ctx context.Context, call Floor_done) error {
	return nil
}



type FloorNode types.StreamNode[any, any]

func NewFloor() FloorNode {
	server := &FloorServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
