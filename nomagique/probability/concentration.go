package probability

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type ConcentrationServer struct {
	Downstream types.Float64Sink
}

func (s *ConcentrationServer) Write(ctx context.Context, payload any) error {
	vals, ok := payload.([]float64)
	if !ok {
		return nil
	}

	var total float64
	for _, v := range vals {
		total += v
	}

	if total == 0 {
		if capnp.Client(s.Downstream).IsValid() {
			return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
				p.SetValue(0.0)
				return nil
			})
		}
		return nil
	}

	var hhi float64
	for _, v := range vals {
		p := v / total
		hhi += p * p
	}

	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(hhi)
			return nil
		})
	}
	return nil
}

func (s *ConcentrationServer) Done(ctx context.Context) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewConcentration() *ConcentrationServer {
	return &ConcentrationServer{}
}
