package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSReadServer struct {
	out []byte
}

func NewWSRead() *WSReadServer {
	return &WSReadServer{}
}

func (server *WSReadServer) Write(ctx context.Context, call WSRead_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSReadServer) Done(ctx context.Context, call WSRead_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wsread: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wsread: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
