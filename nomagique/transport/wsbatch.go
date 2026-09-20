package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSBatchServer struct {
	out []byte
}

func NewWSBatch() *WSBatchServer {
	return &WSBatchServer{}
}

func (server *WSBatchServer) Write(ctx context.Context, call WSBatch_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSBatchServer) Done(ctx context.Context, call WSBatch_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wsbatch: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wsbatch: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
