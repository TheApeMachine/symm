package execution

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type DecideServer struct {
	out string
}

func NewDecide() *DecideServer {
	return &DecideServer{}
}

func (server *DecideServer) Write(ctx context.Context, call Decide_write) error {
	args := call.Args()
	minContrast := args.MinContrast()
	winner, _ := args.Winner()
	contrast := args.Contrast()
	isBreak := args.IsBreak()

	if inData, err := args.In(); err == nil && len(inData) > 0 {
		var eval map[string]any
		if err := sonic.Unmarshal(inData, &eval); err == nil && eval != nil {
			if w, ok := eval["winner"].(string); ok {
				winner = w
			}
			if c, ok := eval["contrast"].(float64); ok {
				contrast = c
			}
			if b, ok := eval["isBreak"].(bool); ok {
				isBreak = b
			}
			if mc, ok := eval["minContrast"].(float64); ok {
				minContrast = mc
			}
		}
	}

	result := "wait"
	if !isBreak && contrast > minContrast && winner != "" {
		result = winner
	}

	server.out = result
	return nil
}

func (server *DecideServer) Done(ctx context.Context, call Decide_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"decide: alloc results failed",
			err,
		))
	}

	results.SetOut(server.out)
	server.out = ""
	return nil
}
