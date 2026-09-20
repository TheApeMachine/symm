package calculus

import (
	"context"
	"math"
)

type AbsoluteServer struct {
	Downstream func(context.Context, float64) error
}

func (s *AbsoluteServer) Write(ctx context.Context, call Absolute_write) error {
	a := call.Args().A()
	result := math.Abs(a)

	return s.Downstream(ctx, result)
}

func (s *AbsoluteServer) Done(ctx context.Context, call Absolute_done) error {
	return nil
}
