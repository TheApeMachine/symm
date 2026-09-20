package statistic

import (
	"context"
)

type EMAServer struct {
	Downstream  func(context.Context, float64) error
	ema         float64
	initialized bool
	Alpha       float64
}

func (s *EMAServer) Write(ctx context.Context, call EMA_write) error {
	a := call.Args().A()
	result := float64(0)
	if s.Alpha <= 0 {
		result = a
	} else if !s.initialized {
		s.ema = a
		s.initialized = true
		result = s.ema
	} else {
		s.ema = (a * s.Alpha) + (s.ema * (1.0 - s.Alpha))
		result = s.ema
	}

	return s.Downstream(ctx, result)
}

func (s *EMAServer) Done(ctx context.Context, call EMA_done) error {
	return nil
}
