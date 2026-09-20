package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"math"
)

type LogServer struct {
	Downstream func(context.Context, float64) error
}

func (s *LogServer) Write(ctx context.Context, call Log_write) error {
	a := call.Args().A()
	result := math.Log(a)

	return s.Downstream(ctx, result)
}

func (s *LogServer) Done(ctx context.Context, call Log_done) error {
	return nil
}



type LogNode types.StreamNode[any, any]

func NewLog() LogNode {
	server := &LogServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
