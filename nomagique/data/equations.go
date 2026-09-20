package data

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type EquationServer struct {
	data []byte
}

func NewEquation() *EquationServer {
	return &EquationServer{}
}

func (s *EquationServer) Write(ctx context.Context, call Equation_write) error {
	in, err := call.Args().In()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read in", err))
	}

	s.data = bytes.Clone(in)
	return nil
}

func (s *EquationServer) Done(ctx context.Context, call Equation_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	if err := results.SetOut(s.data); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to set out", err))
	}

	s.data = nil
	return nil
}
