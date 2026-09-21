package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSCloseServer struct {
	out []byte
}

func NewWSClose() *WSCloseServer {
	return &WSCloseServer{}
}

func (server *WSCloseServer) Write(ctx context.Context, call WSClose_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSCloseServer) Done(ctx context.Context, call WSClose_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wsclose: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wsclose: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
