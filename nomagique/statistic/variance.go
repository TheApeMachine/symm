package statistic

import (
	"context"
)

type VarianceServer struct {
	Downstream func(context.Context, float64) error
	count      float64
	mean       float64
	m2         float64
}

func (s *VarianceServer) Write(ctx context.Context, call Variance_write) error {
	a := call.Args().A()
	s.count++
	delta := a - s.mean
	s.mean += delta / s.count
	delta2 := a - s.mean
	s.m2 += delta * delta2
	result := float64(0)
	if s.count > 1 {
		result = s.m2 / (s.count - 1)
	}

	return s.Downstream(ctx, result)
}

func (s *VarianceServer) Done(ctx context.Context, call Variance_done) error {
	return nil
}
