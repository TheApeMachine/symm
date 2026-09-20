package probability

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type ShannonAmbiguityServer struct {
	vals  []float64
	total float64
}

func NewShannonAmbiguity() *ShannonAmbiguityServer {
	return &ShannonAmbiguityServer{}
}

func (s *ShannonAmbiguityServer) Write(ctx context.Context, call ShannonAmbiguity_write) error {
	val := call.Args().In()
	s.vals = append(s.vals, val)
	s.total += val
	return nil
}

func (s *ShannonAmbiguityServer) Done(ctx context.Context, call ShannonAmbiguity_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	result := float64(0)
	if len(s.vals) >= 2 && s.total != 0 {
		entropy := 0.0
		for _, item := range s.vals {
			p := item / s.total
			if p > 0 {
				entropy -= p * math.Log(p)
			}
		}

		result = entropy / math.Log(float64(len(s.vals)))
	}

	results.SetOut(result)
	s.vals = nil
	s.total = 0
	return nil
}
