package calculus

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type SecondDifferenceServer struct {
	Downstream types.Float64Sink
	v1         float64
	v2         float64
	count      int
}

func (s *SecondDifferenceServer) Write(ctx context.Context, call SecondDifference_write) error {
	a := call.Args().A()
	s.count++
	d := a - s.v1
	d2 := d - s.v2
	s.v2 = d
	s.v1 = a
	result := float64(0)
	if s.count > 2 {
		result = d2
	}

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *SecondDifferenceServer) Done(ctx context.Context, call SecondDifference_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewSecondDifference() *SecondDifferenceServer {
	return &SecondDifferenceServer{}
}
