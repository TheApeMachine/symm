package probability

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type EntropyServer struct {
	out float64
}

func NewEntropy() *EntropyServer {
	return &EntropyServer{}
}

func (s *EntropyServer) Write(ctx context.Context, call Entropy_write) error {
	val := call.Args().In()
	if val > 0 {
		s.out += -val * math.Log(val)
	}

	return nil
}

func (s *EntropyServer) Done(ctx context.Context, call Entropy_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.out = 0
	return nil
}
