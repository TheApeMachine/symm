package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type PolarizeServer struct {
	Downstream func(context.Context, float64) error
}

func (s *PolarizeServer) Write(ctx context.Context, call Polarize_write) error {
	a := call.Args().A()
	b := call.Args().B()
	alpha := a
	if alpha < 0 {
		alpha = 0
	}
	beta := -a
	if beta < 0 {
		beta = 0
	}
	if b > 0 {
		alpha = alpha / (alpha + b)
		beta = beta / (beta + b)
	}
	result := alpha - beta

	return s.Downstream(ctx, result)
}

func (s *PolarizeServer) Done(ctx context.Context, call Polarize_done) error {
	return nil
}



type PolarizeNode types.StreamNode[any, any]

func NewPolarize() PolarizeNode {
	server := &PolarizeServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
