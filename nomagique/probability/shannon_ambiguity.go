package probability

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type ShannonAmbiguityServer struct {
	Downstream types.Float64Sink
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

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *ShannonAmbiguityServer) Done(ctx context.Context, call ShannonAmbiguity_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewShannonAmbiguity() *ShannonAmbiguityServer {
	return &ShannonAmbiguityServer{}
}
