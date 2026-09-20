package sequence

import (
	"context"

	"github.com/theapemachine/symm/nomagique/types"
)

type AtServer struct {
	Downstream func(context.Context, any) error
}

func NewAtServer() *AtServer {
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

type AtNode types.StreamNode[any, any]

func NewAt(index types.Integer) AtNode {
	server := NewAtServer()
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res any) error {
			return next(ctx, res)
		}
	})
}


