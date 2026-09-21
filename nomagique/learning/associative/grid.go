package associative

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
	in, _ := call.Args().Data()
	if len(in) > 0 {
		server.out = bytes.Clone(in)
	} else {
		server.out = []byte("r0")
	}
	return nil
}

func (server *GridServer) Done(ctx context.Context, call Grid_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"associative grid: alloc results failed",
			err,
		))
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"associative grid: set out failed",
			err,
		))
	}

	server.out = nil
	return nil
}
