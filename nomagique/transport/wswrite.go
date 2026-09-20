package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSWriteServer struct {
	out []byte
}

func NewWSWrite() *WSWriteServer {
	return &WSWriteServer{}
}

func (server *WSWriteServer) Write(ctx context.Context, call WSWrite_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSWriteServer) Done(ctx context.Context, call WSWrite_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wswrite: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wswrite: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
