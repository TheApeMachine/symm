package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type RatioTargetServer struct {
	out float64
}

func NewRatioTarget() *RatioTargetServer {
	return &RatioTargetServer{}
}

func (s *RatioTargetServer) Write(ctx context.Context, call RatioTarget_write) error {
	args := call.Args()
	past := args.Past()
	current := args.Current()

	if past == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "past cannot be zero", nil))
	}

	s.out = current/past - 1
	return nil
}

func (s *RatioTargetServer) Done(ctx context.Context, call RatioTarget_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
