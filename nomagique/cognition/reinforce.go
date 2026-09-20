package cognition

import (
	"bytes"
	"context"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

type ReinforceServer struct {
	Root        *atomic.Pointer[iradix.Tree[[]byte]]
	StepCounter *atomic.Uint64
	out         []byte
}

func NewReinforce() *ReinforceServer {
	return &ReinforceServer{}
}

func (s *ReinforceServer) Write(ctx context.Context, call Reinforce_write) error {
	contextBytes, _ := call.Args().ContextBytes()
	classBytes, _ := call.Args().ClassBytes()

	if len(classBytes) > 0 {
		s.out = bytes.Clone(classBytes)
		return nil
	}

	s.out = bytes.Clone(contextBytes)
	return nil
}

func (s *ReinforceServer) Done(ctx context.Context, call Reinforce_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	if err := results.SetOut(s.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to set out", err))
	}

	s.out = nil
	return nil
}
