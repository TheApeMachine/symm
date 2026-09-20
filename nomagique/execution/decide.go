package execution

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"

	"github.com/bytedance/sonic"
)

type DecideServer struct {
	Downstream func(context.Context, string) error
}

func NewDecideServer() *DecideServer {
	return &DecideServer{}
}

func (s *DecideServer) Write(ctx context.Context, call Decide_write) error {
	args, err := call.Args().Decide()
	if err != nil {
		return err
	}
	
	minContrast := args.MinContrast()
	
	evalPtr, err := args.Eval()
	if err != nil || !evalPtr.IsValid() {
		if s.Downstream != nil {
			_ = s.Downstream(ctx, "wait")
		}
		return err
	}

	var eval map[string]any
	evalBytes := evalPtr.Data()
	_ = sonic.Unmarshal(evalBytes, &eval)

	winner := ""
	if w, ok := eval["winner"].(string); ok {
		winner = w
	}
	
	contrast := 0.0
	if c, ok := eval["contrast"].(float64); ok {
		contrast = c
	}
	
	isBreak := false
	if b, ok := eval["isBreak"].(bool); ok {
		isBreak = b
	}

	result := "wait"
	if !isBreak && contrast > minContrast && winner != "" {
		result = winner
	}

	if s.Downstream != nil {
		return s.Downstream(ctx, result)
	}
	return nil
}

func (s *DecideServer) Done(ctx context.Context, call Decide_done) error {
	return nil
}



type DecideNode types.StreamNode[any, any]

func NewDecide() DecideNode {
	server := &DecideServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
