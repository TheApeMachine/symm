package probability

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type GeomeanServer struct {
	count float64
	sum   float64
	out   float64
}

func NewGeomean() *GeomeanServer {
	return &GeomeanServer{}
}

func (s *GeomeanServer) Write(ctx context.Context, call Geomean_write) error {
	val := call.Args().Value()
	if val <= 0 {
		return nil
	}

	s.count++
	s.sum += math.Log(val)
	s.out = math.Exp(s.sum / s.count)
	return nil
}

func (s *GeomeanServer) Done(ctx context.Context, call Geomean_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out)
	s.count = 0
	s.sum = 0
	s.out = 0
	return nil
}
