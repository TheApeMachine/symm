package sequence

import (
	"context"
)

type AtServer struct {
	Downstream func(context.Context, any) error
}

func NewAt() *AtServer {
	return &AtServer{}
}

func (s *AtServer) Evaluate(ctx context.Context, in any) (any, error) {
	if s.Downstream != nil {
		return in, s.Downstream(ctx, in)
	}

	return in, nil
}

func (s *AtServer) Execute(ctx context.Context, call At_execute) error {
	args, err := call.Args().At()
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
