package calculus

import (
	"context"
	"math"
)

type TanhServer struct {
	Downstream func(context.Context, float64) error
}

func (s *TanhServer) Write(ctx context.Context, call Tanh_write) error {
	a := call.Args().A()
	result := math.Tanh(a)

	return s.Downstream(ctx, result)
}

func (s *TanhServer) Done(ctx context.Context, call Tanh_done) error {
	return nil
}
