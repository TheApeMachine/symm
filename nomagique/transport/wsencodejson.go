package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSEncodeJSONServer struct {
	out []byte
}

func NewWSEncodeJSON() *WSEncodeJSONServer {
	return &WSEncodeJSONServer{}
}

func (server *WSEncodeJSONServer) Write(ctx context.Context, call WSEncodeJSON_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSEncodeJSONServer) Done(ctx context.Context, call WSEncodeJSON_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wsencodejson: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wsencodejson: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
