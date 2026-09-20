package calculus

import (
	"context"
)

type SquareServer struct {
	Downstream func(context.Context, float64) error
}

func (s *SquareServer) Write(ctx context.Context, call Square_write) error {
	a := call.Args().A()
	result := a * a

	return s.Downstream(ctx, result)
}

func (s *SquareServer) Done(ctx context.Context, call Square_done) error {
	return nil
}
