package data

import (
	"context"

	"github.com/theapemachine/symm/nomagique/types"
)

type EquationServer struct {
	Downstream func(context.Context, WireMeasurement) error
}

func NewEquationServer() *EquationServer {
	return &EquationServer{}
}

func (s *EquationServer) Evaluate(ctx context.Context, measurement WireMeasurement) (WireMeasurement, error) {
	if s.Downstream != nil {
		return measurement, s.Downstream(ctx, measurement)
	}
	return measurement, nil
}

func (s *EquationServer) Write(ctx context.Context, call Equation_write) error {
	measurement, err := call.Args().Measurement()
	if err != nil {
		return err
	}
	_, err = s.Evaluate(ctx, measurement)
	return err
}

func (s *EquationServer) Done(ctx context.Context, call Equation_done) error {
	return nil
}

type EquationNode types.StreamNode[WireMeasurement, WireMeasurement]

func NewEquationNode() EquationNode {
	server := &EquationServer{}
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		input := in.(WireMeasurement)
		_, err := server.Evaluate(ctx, input)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res WireMeasurement) error {
			return next(ctx, res)
		}
	})
}
