package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type FanServer struct {
	out []byte
}

func NewFan() *FanServer {
	return &FanServer{}
}

func (server *FanServer) Write(ctx context.Context, call Fan_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *FanServer) Done(ctx context.Context, call Fan_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"fan: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"fan: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
