package data

import (
	"context"
)

type EquationServer struct {
	Downstream func(context.Context, WireMeasurement) error
}

func NewEquation() *EquationServer {
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
