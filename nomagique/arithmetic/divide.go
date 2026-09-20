package arithmetic

import (
	"context"
)

type DivideServer struct {
	Downstream func(context.Context, float64) error
}

func (s *DivideServer) Write(ctx context.Context, call Divide_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := a / b
	return s.Downstream(ctx, result)
}

func (s *DivideServer) Done(ctx context.Context, call Divide_done) error {
	return nil
}
