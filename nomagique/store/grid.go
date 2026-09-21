package store

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type GridServer struct {
	out []byte
}

func NewGrid() *GridServer {
	return &GridServer{}
}

func (server *GridServer) Write(ctx context.Context, call Grid_write) error {
	inData, err := call.Args().Data()
	if err == nil && len(inData) > 0 {
		server.out = bytes.Clone(inData)
	}

	return nil
}

func (server *GridServer) Done(ctx context.Context, call Grid_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"grid: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"grid: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
