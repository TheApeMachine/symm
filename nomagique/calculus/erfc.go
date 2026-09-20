package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type ErfcServer struct {
	Downstream func(context.Context, float64) error
}

func (s *ErfcServer) Write(ctx context.Context, call Erfc_write) error {
	a := call.Args().A()
	result := math.Erfc(a)

	return s.Downstream(ctx, result)
}

func (s *ErfcServer) Done(ctx context.Context, call Erfc_done) error {
	return nil
}



type ErfcNode types.StreamNode[any, any]

func NewErfc() ErfcNode {
	server := &ErfcServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
