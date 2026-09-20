package calculus

import (
	"context"
)

type SignServer struct {
	Downstream func(context.Context, float64) error
}

func (s *SignServer) Write(ctx context.Context, call Sign_write) error {
	a := call.Args().A()
	result := float64(0)
	if a < 0 {
		result = -1
	} else if a > 0 {
		result = 1
	}

	return s.Downstream(ctx, result)
}

func (s *SignServer) Done(ctx context.Context, call Sign_done) error {
	return nil
}
