package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type ForkServer struct {
	out []byte
}

func NewFork() *ForkServer {
	return &ForkServer{}
}

func (server *ForkServer) Write(ctx context.Context, call Fork_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *ForkServer) Done(ctx context.Context, call Fork_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"fork: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"fork: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
