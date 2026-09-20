package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
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



type ThresholdNode types.StreamNode[any, any]

func NewThreshold() ThresholdNode {
	server := &ThresholdServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
