package calculus

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type LogServer struct {
	Downstream types.Float64Sink
}

func (s *LogServer) Write(ctx context.Context, call Log_write) error {
	a := call.Args().A()
	result := math.Log(a)
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetValue(result)
			return nil
		})
	}
	return nil
}

func (s *LogServer) Done(ctx context.Context, call Log_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func NewLog() *LogServer {
	return &LogServer{}
}
