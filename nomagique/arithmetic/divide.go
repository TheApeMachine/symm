package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

type DivideServer struct {
	Downstream func(context.Context, float64) error
}

func (s *DivideServer) Write(ctx context.Context, call Divide_write) error {
	a := call.Args().A()
	b := call.Args().B()
	result := a / b
	return s.Downstream(ctx, result)
}

func (s *DivideServer) Done(ctx context.Context, call Divide_done) error {
	return nil
}



type DivideNode types.StreamNode[any, any]

func NewDivide() DivideNode {
	server := &DivideServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
