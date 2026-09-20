package calculus

import (
	"context"
	"math"
)

type ExpServer struct {
	Downstream func(context.Context, float64) error
}

func (s *ExpServer) Write(ctx context.Context, call Exp_write) error {
	a := call.Args().A()
	result := math.Exp(a)

	return s.Downstream(ctx, result)
}

func (s *ExpServer) Done(ctx context.Context, call Exp_done) error {
	return nil
}
