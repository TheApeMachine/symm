package learning

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type DirectionalTargetServer struct {
	Deadband float64
	out      float64
}

func NewDirectionalTarget() *DirectionalTargetServer {
	return &DirectionalTargetServer{}
}

func (s *DirectionalTargetServer) Write(ctx context.Context, call DirectionalTarget_write) error {
	args := call.Args()
	past := args.Past()
	current := args.Current()
	delta := current - past

	var result float64
	if math.Abs(delta) > s.Deadband {
		result = math.Copysign(1, delta)
	}

	s.out = result
	return nil
}

func (s *DirectionalTargetServer) Done(ctx context.Context, call DirectionalTarget_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
