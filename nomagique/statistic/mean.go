package statistic

import (
	"context"
)

type MeanServer struct {
	Downstream func(context.Context, float64) error
	count      float64
	sum        float64
}

func (s *MeanServer) Write(ctx context.Context, call Mean_write) error {
	a := call.Args().A()
	s.count++
	s.sum += a
	result := s.sum / s.count

	return s.Downstream(ctx, result)
}

func (s *MeanServer) Done(ctx context.Context, call Mean_done) error {
	return nil
}
