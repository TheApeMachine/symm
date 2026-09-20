package execution

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type submitState struct {
	symbol    string
	action    string
	status    string
	timestamp int64
}

type SubmitServer struct {
	state submitState
}

func NewSubmit() *SubmitServer {
	return &SubmitServer{}
}

func (s *SubmitServer) Write(ctx context.Context, call Submit_write) error {
	args := call.Args()
	action, err := args.Action()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "failed to read action", err))
	}

	symbol, _ := args.Symbol()
	if symbol == "" {
		symbol = "DEFAULT"
	}

	if action != "enter" && action != "exit" {
		return nil
	}

	s.state = submitState{
		symbol:    symbol,
		action:    action,
		status:    "SUBMITTED",
		timestamp: time.Now().UnixNano(),
	}
	return nil
}

func (s *SubmitServer) Done(ctx context.Context, call Submit_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	orderBytes, err := sonic.Marshal(map[string]any{
		"symbol":    s.state.symbol,
		"action":    s.state.action,
		"status":    s.state.status,
		"timestamp": s.state.timestamp,
	})
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to marshal order", err))
	}

	_ = results.SetOut(orderBytes)
	_ = results.SetSymbol(s.state.symbol)
	_ = results.SetAction(s.state.action)
	_ = results.SetStatus(s.state.status)
	results.SetTimestamp(s.state.timestamp)
	s.state = submitState{}
	return nil
}
