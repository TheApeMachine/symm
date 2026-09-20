package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type DeltaTargetServer struct {
	out float64
}

func NewDeltaTarget() *DeltaTargetServer {
	return &DeltaTargetServer{}
}

func (s *DeltaTargetServer) Write(ctx context.Context, call DeltaTarget_write) error {
	args := call.Args()
	past := args.Past()
	current := args.Current()

	s.out = current - past
	return nil
}

func (s *DeltaTargetServer) Done(ctx context.Context, call DeltaTarget_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
