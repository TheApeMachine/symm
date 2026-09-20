package arithmetic

import (
	"context"
)

type MultiplyServer struct {
	Downstream func(context.Context, float64) error
}

func (s *MultiplyServer) Write(ctx context.Context, call Multiply_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := a * b
	return s.Downstream(ctx, result)
}

func (s *MultiplyServer) Done(ctx context.Context, call Multiply_done) error {
	return nil
}
