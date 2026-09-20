package probability

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type GeomeanServer struct {
	Downstream types.Float64Sink
	count      float64
	sum        float64
}

func (s *GeomeanServer) Write(ctx context.Context, call Geomean_write) error {
	a := call.Args().A()
	s.count++
	s.sum += math.Log(a)
	result := math.Exp(s.sum / s.count)
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *GeomeanServer) Done(ctx context.Context, call Geomean_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewGeomean() *GeomeanServer {
	return &GeomeanServer{}
}
