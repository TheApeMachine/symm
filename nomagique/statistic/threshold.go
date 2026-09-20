package statistic

import (
	"context"
)

type ThresholdServer struct {
	Downstream func(context.Context, float64) error
	Band       float64
	Rest       float64
	Lower      float64
	Upper      float64
}

func (s *ThresholdServer) Write(ctx context.Context, call Threshold_write) error {
	a := call.Args().A()
	result := s.Rest
	if a < s.Band {
		result = s.Upper
	} else if a > 1.0-s.Band {
		result = s.Lower
	}

	return s.Downstream(ctx, result)
}

func (s *ThresholdServer) Done(ctx context.Context, call Threshold_done) error {
	return nil
}
