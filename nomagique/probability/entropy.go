package probability

import (
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type EntropyServer struct {
	Downstream func(context.Context, float64) error
	acc        float64
}

func (s *EntropyServer) Write(ctx context.Context, call Entropy_write) error {
	m := call.Args().A()

	if m != 0 {
		s.acc += -m * math.Log(m)
	}
	result := s.acc

	return s.Downstream(ctx, result)
}

func (s *EntropyServer) Done(ctx context.Context, call Entropy_done) error {
	return nil
}

type EntropyNode types.StreamNode[any, any]

func NewEntropy() EntropyNode {
	server := &EntropyServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {
			server.Downstream = func(c context.Context, p float64) error {
				return next(c, p)
			}
		},
	)
}
