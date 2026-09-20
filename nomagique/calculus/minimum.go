package calculus

import (
	"context"
	"math"
)

type MinimumServer struct {
	Downstream func(context.Context, float64) error
}

func (s *MinimumServer) Write(ctx context.Context, call Minimum_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := math.Min(a, b)

	return s.Downstream(ctx, result)
}

func (s *MinimumServer) Done(ctx context.Context, call Minimum_done) error {
	return nil
}
