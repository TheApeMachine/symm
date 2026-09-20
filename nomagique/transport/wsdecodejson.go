package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSDecodeJSONServer struct {
	out []byte
}

func NewWSDecodeJSON() *WSDecodeJSONServer {
	return &WSDecodeJSONServer{}
}

func (server *WSDecodeJSONServer) Write(ctx context.Context, call WSDecodeJSON_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSDecodeJSONServer) Done(ctx context.Context, call WSDecodeJSON_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wsdecodejson: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wsdecodejson: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
