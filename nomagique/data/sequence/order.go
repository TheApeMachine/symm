package sequence

import (
	"context"
)

type OrderServer struct {
	Downstream func(context.Context, any) error
}

func NewOrder() *OrderServer {
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
