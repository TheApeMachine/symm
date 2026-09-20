package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type AtanhServer struct {
	Downstream func(context.Context, float64) error
}

func (s *AtanhServer) Write(ctx context.Context, call Atanh_write) error {
	a := call.Args().A()
	result := math.Atanh(a)

	return s.Downstream(ctx, result)
}

func (s *AtanhServer) Done(ctx context.Context, call Atanh_done) error {
	return nil
}



type AtanhNode types.StreamNode[any, any]

func NewAtanh() AtanhNode {
	server := &AtanhServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
