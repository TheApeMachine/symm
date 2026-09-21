package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type GateServer struct {
	out []byte
}

func NewGate() *GateServer {
	return &GateServer{}
}

func (server *GateServer) Write(ctx context.Context, call Gate_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *GateServer) Done(ctx context.Context, call Gate_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"gate: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"gate: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
