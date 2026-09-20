package temporal

import (
	"context"
	"math"
)

type LogReturnsServer struct {
	Downstream  func(context.Context, float64) error
	previous    float64
	initialized bool
}

func (s *LogReturnsServer) Write(ctx context.Context, call LogReturns_write) error {
	a := call.Args().A()
	result := float64(0)
	if !s.initialized || s.previous <= 0 || a <= 0 {
		s.previous = a
		s.initialized = true
	} else {
		result = math.Log(a / s.previous)
		s.previous = a
	}
	return s.Downstream(ctx, result)
}

func (s *LogReturnsServer) Done(ctx context.Context, call LogReturns_done) error {
	return nil
}
