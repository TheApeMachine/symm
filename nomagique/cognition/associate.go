package cognition

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type AssociateServer struct {
	prec       []byte
	outPrec    []byte
	outCurrent []byte
}

func NewAssociate() *AssociateServer {
	return &AssociateServer{}
}

func (s *AssociateServer) Write(ctx context.Context, call Associate_write) error {
	current, err := call.Args().Current()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read current", err))
	}

	if len(current) == 0 {
		return nil
	}

	if len(s.prec) == 0 {
		s.prec = bytes.Clone(current)
		s.outPrec = bytes.Clone(current)
		s.outCurrent = nil
		return nil
	}

	s.outPrec = bytes.Clone(s.prec)
	s.outCurrent = bytes.Clone(current)
	s.prec = bytes.Clone(current)
	return nil
}

func (s *AssociateServer) Done(ctx context.Context, call Associate_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	if err := results.SetPrecursor(s.outPrec); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to set precursor", err))
	}

	if err := results.SetCurrent(s.outCurrent); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to set current", err))
	}

	s.outPrec = nil
	s.outCurrent = nil
	return nil
}
