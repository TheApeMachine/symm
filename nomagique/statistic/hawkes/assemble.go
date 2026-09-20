package hawkes

import (
	"context"
	"fmt"
	"github.com/bytedance/sonic"
)

type AssembleServer struct {
	Downstream func(context.Context, [2]float64) error
}

func NewAssembleServer() *AssembleServer {
	return &AssembleServer{}
}

func (s *AssembleServer) Write(ctx context.Context, call Assemble_write) error {
	args, err := call.Args().Assemble()
	if err != nil {
		return err
	}
	
	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}

	if s.Downstream == nil {
		return nil
	}
	
	var in map[string]any
	if payloadPtr.IsValid() {
		data := payloadPtr.Data()
		if len(data) > 0 {
			_ = sonic.Unmarshal(data, &in)
		}
	}
	
	ts := 0.0
	sideStr := ""
	
	if in != nil {
		if t, ok := in["timestamp"].(float64); ok {
			ts = t
		} else if t, ok := in["timestamp"].(int64); ok {
			ts = float64(t)
		} else if t, ok := in["time"].(float64); ok {
			ts = t
		} else if t, ok := in["time"].(int64); ok {
			ts = float64(t)
		}
		
		if sv, ok := in["side"].(string); ok {
			sideStr = sv
		} else if sideAny, exists := in["side"]; exists {
			sideStr = fmt.Sprint(sideAny)
		}
	}
	
	mark := 1.0
	if sideStr == "sell" || sideStr == "s" {
		mark = -1.0
	}
	
	result := [2]float64{ts, mark}
	return s.Downstream(ctx, result)
}

func (s *AssembleServer) Done(ctx context.Context, call Assemble_done) error {
	return nil
}
