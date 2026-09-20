package calculus

import (
	"context"
	"math"
)

type LogServer struct {
	Downstream func(context.Context, float64) error
}

func (s *LogServer) Write(ctx context.Context, call Log_write) error {
	a := call.Args().A()
	result := math.Log(a)

	return s.Downstream(ctx, result)
}

func (s *LogServer) Done(ctx context.Context, call Log_done) error {
	return nil
}
