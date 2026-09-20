package probability

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type EntropyServer struct {
	Downstream types.Float64Sink
	acc        float64
}

func (s *EntropyServer) Write(ctx context.Context, call Entropy_write) error {
	m := call.Args().A()
	if m != 0 {
		s.acc += -m * math.Log(m)
	}

	result := s.acc
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *EntropyServer) Done(ctx context.Context, call Entropy_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewEntropy() *EntropyServer {
	return &EntropyServer{}
}
