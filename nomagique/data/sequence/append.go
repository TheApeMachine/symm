package sequence

import (
	"context"

	"github.com/theapemachine/symm/nomagique/types"
)

type AppendServer struct {
	Downstream func(context.Context, any) error
}

func NewAppendServer() *AppendServer {
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

type AppendNode types.StreamNode[any, any]

func NewAppendNode() AppendNode {
	server := NewAppendServer()
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res any) error {
			return next(ctx, res)
		}
	})
}
