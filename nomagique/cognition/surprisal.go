package cognition

import (
	"context"
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

type SurprisalServer struct {
	Root        *atomic.Pointer[iradix.Tree[[]byte]]
	StepCounter *atomic.Uint64
	out         float64
}

func NewSurprisal() *SurprisalServer {
	return &SurprisalServer{}
}

func (s *SurprisalServer) Write(ctx context.Context, call Surprisal_write) error {
	contextBytes, _ := call.Args().ContextBytes()
	s.out = 0.0

	if s.Root == nil || len(contextBytes) == 0 {
		return nil
	}

	tree := s.Root.Load()
	if tree == nil {
		return nil
	}

	var step uint64
	if s.StepCounter != nil {
		step = s.StepCounter.Load()
	}

	totalSteps := float64(step)
	if totalSteps <= 0 {
		return nil
	}

	sensoryKey := make([]byte, 2+len(contextBytes))
	sensoryKey[0] = 's'
	sensoryKey[1] = '/'
	copy(sensoryKey[2:], contextBytes)
	raw, found := tree.Get(sensoryKey)
	if !found || len(raw) < 24 {
		return nil
	}

	prob := 1.0 / totalSteps
	if prob > 0 {
		s.out = -math.Log2(prob)
	}

	return nil
}

func (s *SurprisalServer) Done(ctx context.Context, call Surprisal_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
