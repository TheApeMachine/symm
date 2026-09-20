package calculus

import (
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
