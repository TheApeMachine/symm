package sequence

import (
	"context"
)

type AppendServer struct {
	Downstream func(context.Context, any) error
}

func NewAppend() *AppendServer {
	return &AppendServer{}
}

func (s *AppendServer) Evaluate(ctx context.Context, in any) (any, error) {
	if s.Downstream != nil {
		return in, s.Downstream(ctx, in)
	}

	return in, nil
}

func (s *AppendServer) Execute(ctx context.Context, call Append_execute) error {
	args, err := call.Args().Append()
	if err != nil {
		return err
	}

	_, err = args.Item()
	if err != nil {
		return err
	}

	_, err = s.Evaluate(ctx, nil)
	return err
}
