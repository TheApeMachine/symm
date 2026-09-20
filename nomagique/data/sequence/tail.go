package sequence

import (
	"context"

	"github.com/theapemachine/symm/nomagique/types"
)

type TailServer struct {
	size       int
	Downstream func(context.Context, any) error
}

func NewTailServer(size int) *TailServer {
	return &TailServer{size: size}
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

type TailNode types.StreamNode[any, any]

func NewTail(size types.Integer) TailNode {
	sz := 10
	if size != nil {
		sz = size(nil)
	}
	server := NewTailServer(sz)
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		_, err := server.Evaluate(ctx, in)
		return err
	}, func(next func(context.Context, any) error) {
		server.Downstream = func(ctx context.Context, res any) error {
			return next(ctx, res)
		}
	})
}


