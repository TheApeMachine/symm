package learning

import (
	"context"

	"github.com/theapemachine/errnie"
)

type IdentityTargetServer struct {
	out float64
}

func NewIdentityTarget() *IdentityTargetServer {
	return &IdentityTargetServer{}
}

func (s *IdentityTargetServer) Write(ctx context.Context, call IdentityTarget_write) error {
	s.out = call.Args().Current()
	return nil
}

func (s *IdentityTargetServer) Done(ctx context.Context, call IdentityTarget_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
