package probability

import (
	"context"
)

type GeomeanServer struct {
	Downstream func(context.Context, float64) error
}

func (s *GeomeanServer) Write(ctx context.Context, call Geomean_write) error {
	a := call.Args().A()
	result := a

	return s.Downstream(ctx, result)
}

func (s *GeomeanServer) Done(ctx context.Context, call Geomean_done) error {
	return nil
}
