package sequence

import (
	"context"
)

type TailServer struct {
	size       int
	Downstream func(context.Context, any) error
}

func NewTail() *TailServer {
	return &TailServer{size: 10}
}

func (s *TailServer) Evaluate(ctx context.Context, in any) (any, error) {
	if s.Downstream != nil {
		return in, s.Downstream(ctx, in)
	}

	return in, nil
}

func (s *TailServer) Execute(ctx context.Context, call Tail_execute) error {
	args, err := call.Args().Tail()
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
