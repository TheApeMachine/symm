package statistic

import (
	"context"
)

type CausalMeanServer struct {
	Downstream func(context.Context, float64) error
	count      float64
	sum        float64
	prevMean   float64
}

func (s *CausalMeanServer) Write(ctx context.Context, call CausalMean_write) error {
	a := call.Args().A()
	ret := s.prevMean
	s.count++
	s.sum += a
	s.prevMean = s.sum / s.count
	result := ret
	if s.count == 1 {
		result = a
	}

	return s.Downstream(ctx, result)
}

func (s *CausalMeanServer) Done(ctx context.Context, call CausalMean_done) error {
	return nil
}
