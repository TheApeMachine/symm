package hawkes

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type AssembleServer struct {
	out [2]float64
}

func NewAssemble() *AssembleServer {
	return &AssembleServer{}
}

func (s *AssembleServer) Write(ctx context.Context, call Assemble_write) error {
	args := call.Args()
	ts := float64(args.Timestamp())
	sideStr, _ := args.Side()

	if inData, err := args.In(); err == nil && len(inData) > 0 {
		var in map[string]any
		if err := sonic.Unmarshal(inData, &in); err == nil && in != nil {
			if t, ok := in["timestamp"].(float64); ok {
				ts = t
			}
			if t, ok := in["timestamp"].(int64); ok {
				ts = float64(t)
			}
			if t, ok := in["time"].(float64); ok {
				ts = t
			}
			if t, ok := in["time"].(int64); ok {
				ts = float64(t)
			}
			if sv, ok := in["side"].(string); ok {
				sideStr = sv
			}
		}
	}

	mark := 1.0
	if sideStr == "sell" || sideStr == "s" {
		mark = -1.0
	}

	s.out = [2]float64{ts, mark}
	return nil
}

func (s *AssembleServer) Done(ctx context.Context, call Assemble_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	results.SetOut(s.out[0])
	results.SetTime(s.out[0])
	results.SetMark(s.out[1])
	s.out = [2]float64{}
	return nil
}
