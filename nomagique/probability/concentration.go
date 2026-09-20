package probability

import (
	"context"
	
	"github.com/theapemachine/symm/nomagique/types"
)

type ConcentrationNode types.StreamNode[any, any]

type ConcentrationServer struct {
	Downstream func(context.Context, any) error
}

func (s *ConcentrationServer) Write(ctx context.Context, payload any) error {
	vals, ok := payload.([]float64)
	if !ok {
		return s.Downstream(ctx, payload)
	}

	var total float64
	for _, v := range vals {
		total += v
	}

	if total == 0 {
		return s.Downstream(ctx, 0.0)
	}

	var hhi float64
	for _, v := range vals {
		p := v / total
		hhi += p * p
	}

	return s.Downstream(ctx, hhi)
}

func NewConcentration() ConcentrationNode {
	server := &ConcentrationServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return server.Write(ctx, payload)
		},
		func(next func(context.Context, any) error) {
			server.Downstream = next
		},
	)
}
