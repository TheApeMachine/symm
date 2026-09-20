package cognition

import (
	"context"
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

type SurprisalServer struct {
	Downstream  func(context.Context, float64) error
	Root        *atomic.Pointer[iradix.Tree[[]byte]]
	StepCounter *atomic.Uint64
}

func (s *SurprisalServer) Write(ctx context.Context, call Surprisal_write) error {
	contextBytes, _ := call.Args().ContextBytes()
	result := float64(0)
	if s.Root == nil || len(contextBytes) == 0 {
		return s.Downstream(ctx, result)
	}
	tree := s.Root.Load()
	if tree == nil {
		return s.Downstream(ctx, result)
	}
	var step uint64
	if s.StepCounter != nil {
		step = s.StepCounter.Load()
	}
	totalSteps := float64(step)
	if totalSteps <= 0 {
		return s.Downstream(ctx, result)
	}
	sensoryKey := make([]byte, 2+len(contextBytes))
	sensoryKey[0] = 's'
	sensoryKey[1] = '/'
	copy(sensoryKey[2:], contextBytes)
	raw, found := tree.Get(sensoryKey)
	if !found || len(raw) < 24 {
		return s.Downstream(ctx, result)
	}
	// TODO: decode weight and get prob
	prob := 1.0 / totalSteps
	if prob > 0 {
		result = -math.Log2(prob)
	}
	return s.Downstream(ctx, result)
}

func (s *SurprisalServer) Done(ctx context.Context, call Surprisal_done) error {
	return nil
}

func NewSurprisal() *SurprisalServer {
	return &SurprisalServer{}
}
