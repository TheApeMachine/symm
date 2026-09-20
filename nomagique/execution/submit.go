package execution

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"time"
)

type SubmitServer struct {
	Downstream func(context.Context, map[string]any) error
}

func NewSubmitServer() *SubmitServer {
	return &SubmitServer{}
}

func (s *SubmitServer) Write(ctx context.Context, call Submit_write) error {
	args, err := call.Args().Submit()
	if err != nil {
		return err
	}
	
	action, err := args.Action()
	if err != nil {
		return err
	}
	
	symbol, err := args.Symbol()
	if err != nil {
		return err
	}

	if action != "enter" && action != "exit" {
		return nil
	}

	intent := map[string]any{
		"symbol":    symbol,
		"action":    action,
		"status":    "SUBMITTED",
		"timestamp": time.Now().UnixNano(),
	}

	if s.Downstream != nil {
		return s.Downstream(ctx, intent)
	}
	return nil
}

func (s *SubmitServer) Done(ctx context.Context, call Submit_done) error {
	return nil
}



type SubmitNode types.StreamNode[any, any]

func NewSubmit() SubmitNode {
	server := &SubmitServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
