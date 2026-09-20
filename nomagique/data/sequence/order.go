package sequence

import (
	"context"

	"github.com/theapemachine/symm/nomagique/types"
)

type OrderServer struct {
	Downstream func(context.Context, any) error
}

func NewOrderServer() *OrderServer {
	return &OrderServer{}
}

func (s *OrderServer) Evaluate(ctx context.Context, in any) (any, error) {
	if s.Downstream != nil {
		return in, s.Downstream(ctx, in)
	}
	return in, nil
}

func (s *OrderServer) Execute(ctx context.Context, call Order_execute) error {
	args, err := call.Args().Order()
	if err != nil {
		return err
	}
	
	_, err = args.Payloads()
	if err != nil {
		return err
	}
	
	_, err = s.Evaluate(ctx, nil)
	return err
}

type OrderNode types.StreamNode[any, any]

func NewOrder() OrderNode {
	server := NewOrderServer()
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res any) error {
			return next(ctx, res)
		}
	})
}
