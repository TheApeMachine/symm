package learning

import (
	"context"
)

type BinaryTargetServer struct {
	DownstreamBinaryTarget func(context.Context, float64) error
}

func (s *BinaryTargetServer) Write(ctx context.Context, call BinaryTarget_write) error {
	return s.WriteParams(ctx, call.Args())
}

func (s *BinaryTargetServer) WriteParams(ctx context.Context, callArgs BinaryTarget_write_Params) error {
	past := callArgs.Past()
	current := callArgs.Current()
	var result float64
	if current > past {
		result = 1.0
	}
	if s.DownstreamBinaryTarget != nil {
		return s.DownstreamBinaryTarget(ctx, result)
	}
	return nil
}

func (s *BinaryTargetServer) Done(ctx context.Context, call BinaryTarget_done) error {
	return nil
}

func NewBinaryTarget() *BinaryTargetServer {
	return &BinaryTargetServer{}
}
