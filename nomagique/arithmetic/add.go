package arithmetic

import (
	"context"
)

type AddServer struct {
	Downstream func(context.Context, float64) error
}

func (s *AddServer) Write(ctx context.Context, call Add_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := a + b
	return s.Downstream(ctx, result)
}

func (s *AddServer) Done(ctx context.Context, call Add_done) error {
	return nil
}
