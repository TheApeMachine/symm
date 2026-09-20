package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type BroadcastServer struct {
	out []byte
}

func NewBroadcast() *BroadcastServer {
	return &BroadcastServer{}
}

func (server *BroadcastServer) Write(ctx context.Context, call Broadcast_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *BroadcastServer) Done(ctx context.Context, call Broadcast_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"broadcast: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"broadcast: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
