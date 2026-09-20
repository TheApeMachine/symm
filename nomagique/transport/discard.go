package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type DiscardServer struct {
	out []byte
}

func NewDiscard() *DiscardServer {
	return &DiscardServer{}
}

func (server *DiscardServer) Write(ctx context.Context, call Discard_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *DiscardServer) Done(ctx context.Context, call Discard_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"discard: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"discard: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
