package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type TeeServer struct {
	out []byte
}

func NewTee() *TeeServer {
	return &TeeServer{}
}

func (server *TeeServer) Write(ctx context.Context, call Tee_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *TeeServer) Done(ctx context.Context, call Tee_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"tee: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"tee: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
