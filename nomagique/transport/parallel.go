package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type ParallelServer struct {
	out []byte
}

func NewParallel() *ParallelServer {
	return &ParallelServer{}
}

func (server *ParallelServer) Write(ctx context.Context, call Parallel_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *ParallelServer) Done(ctx context.Context, call Parallel_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"parallel: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"parallel: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
