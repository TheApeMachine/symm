package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSPingPongServer struct {
	out []byte
}

func NewWSPingPong() *WSPingPongServer {
	return &WSPingPongServer{}
}

func (server *WSPingPongServer) Write(ctx context.Context, call WSPingPong_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSPingPongServer) Done(ctx context.Context, call WSPingPong_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wspingpong: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wspingpong: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
