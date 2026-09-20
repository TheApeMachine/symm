package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type BinaryTargetServer struct {
	out float64
}

func NewBinaryTarget() *BinaryTargetServer {
	return &BinaryTargetServer{}
}

func (s *BinaryTargetServer) Write(ctx context.Context, call BinaryTarget_write) error {
	args := call.Args()
	past := args.Past()
	current := args.Current()

	var result float64
	if current > past {
		result = 1.0
	}

	s.out = result
	return nil
}

func (s *BinaryTargetServer) Done(ctx context.Context, call BinaryTarget_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
