package calculus

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type BoundServer struct {
	Downstream types.Float64Sink
	Min        float64
	Max        float64
}

func (s *BoundServer) Write(ctx context.Context, call Bound_write) error {
	a := call.Args().A()
	result := math.Max(s.Min, math.Min(s.Max, a))
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *BoundServer) Done(ctx context.Context, call Bound_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewBound() *BoundServer {
	return &BoundServer{}
}
