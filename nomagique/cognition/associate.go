package cognition

import (
	"bytes"
	"context"
)

type AssociateServer struct {
	Downstream func(context.Context, []byte, []byte) error
	prec       []byte
}

func (s *AssociateServer) Write(ctx context.Context, call Associate_write) error {
	current, _ := call.Args().Current()
	if len(current) == 0 {
		return nil
	}
	if len(s.prec) == 0 {
		s.prec = bytes.Clone(current)
		return s.Downstream(ctx, bytes.Clone(current), nil)
	}
	outPrecursor := bytes.Clone(s.prec)
	outCurrent := bytes.Clone(current)
	s.prec = bytes.Clone(current)
	return s.Downstream(ctx, outPrecursor, outCurrent)
}

func (s *AssociateServer) Done(ctx context.Context, call Associate_done) error {
	return nil
}

func NewAssociate() *AssociateServer {
	return &AssociateServer{}
}
