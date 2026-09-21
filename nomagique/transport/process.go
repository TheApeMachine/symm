package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type ProcessServer struct {
	*runtime.System
	out []byte
}

func NewProcess(ctx context.Context) *ProcessServer {
	return &ProcessServer{
		System: runtime.NewSystem(ctx, "transport.process"),
	}
}

func (server *ProcessServer) Write(ctx context.Context, call Process_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *ProcessServer) Done(ctx context.Context, call Process_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"process: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"process: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
