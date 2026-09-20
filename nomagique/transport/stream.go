package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type StreamServer struct {
	out []byte
}

func NewStream() *StreamServer {
	return &StreamServer{}
}

func (server *StreamServer) Write(ctx context.Context, call Stream_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *StreamServer) Done(ctx context.Context, call Stream_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"stream: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"stream: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
