package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type ProcessServer struct {
	out []byte
}

func NewProcess() *ProcessServer {
	return &ProcessServer{}
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
