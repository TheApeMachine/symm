package probability

import (
	"context"
	"github.com/theapemachine/symm/nomagique/types"
	"math"
)

type ShannonAmbiguityServer struct {
	Downstream func(context.Context, float64) error
	vals       []float64
	total      float64
}

func (s *ShannonAmbiguityServer) Write(ctx context.Context, call ShannonAmbiguity_write) error {
	a := call.Args().A()
	s.vals = append(s.vals, a)
	s.total += a
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

	return s.Downstream(ctx, result)
}

func (s *ShannonAmbiguityServer) Done(ctx context.Context, call ShannonAmbiguity_done) error {
	return nil
}

type ShannonAmbiguityNode types.StreamNode[any, any]

func NewShannonAmbiguity() ShannonAmbiguityNode {
	server := &ShannonAmbiguityServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
