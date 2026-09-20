package calculus

import (
	"context"
	"math"
)

type ErfcServer struct {
	Downstream func(context.Context, float64) error
}

func (s *ErfcServer) Write(ctx context.Context, call Erfc_write) error {
	a := call.Args().A()
	result := math.Erfc(a)

	return s.Downstream(ctx, result)
}

func (s *ErfcServer) Done(ctx context.Context, call Erfc_done) error {
	return nil
}
