package probability

import (
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type GeomeanServer struct {
	Downstream func(context.Context, float64) error
	count      float64
	sum        float64
}

func (s *GeomeanServer) Write(ctx context.Context, call Geomean_write) error {
	a := call.Args().A()
	
	s.count++
	s.sum += math.Log(a)
	result := math.Exp(s.sum / s.count)

	return s.Downstream(ctx, result)
}

func (s *GeomeanServer) Done(ctx context.Context, call Geomean_done) error {
	return nil
}

type GeomeanNode types.StreamNode[any, any]

func NewGeomean() GeomeanNode {
	server := &GeomeanServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
