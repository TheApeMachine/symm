package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type ReciprocalServer struct {
	Downstream func(context.Context, float64) error
}

func (s *ReciprocalServer) Write(ctx context.Context, call Reciprocal_write) error {
	a := call.Args().A()
	result := float64(0)
	if a != 0 {
		result = 1.0 / a
	}

	return s.Downstream(ctx, result)
}

func (s *ReciprocalServer) Done(ctx context.Context, call Reciprocal_done) error {
	return nil
}



type ReciprocalNode types.StreamNode[any, any]

func NewReciprocal() ReciprocalNode {
	server := &ReciprocalServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
