package sequence

import (
	"context"
)

type ValuesServer struct {
	Downstream func(context.Context, any) error
}

func NewValues() *ValuesServer {
	return &ValuesServer{}
}

func (s *ValuesServer) Evaluate(ctx context.Context, in any) (any, error) {
	if s.Downstream != nil {
		return in, s.Downstream(ctx, in)
	}

	return in, nil
}

func (s *ValuesServer) Execute(ctx context.Context, call Values_execute) error {
	args, err := call.Args().Values()
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
