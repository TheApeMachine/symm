package sequence

import (
	"context"

	"github.com/theapemachine/symm/nomagique/types"
)

type ValuesServer struct {
	Downstream func(context.Context, any) error
}

func NewValuesServer() *ValuesServer {
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

type ValuesNode types.StreamNode[any, any]

func NewValuesNode() ValuesNode {
	server := NewValuesServer()
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res any) error {
			return next(ctx, res)
		}
	})
}
