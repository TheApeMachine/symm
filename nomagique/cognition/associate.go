package cognition

import (
	"github.com/theapemachine/symm/nomagique/types"
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



type AssociateNode types.StreamNode[any, any]

func NewAssociate() AssociateNode {
	server := &AssociateServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
