package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSJSONMessageServer struct {
	out []byte
}

func NewWSJSONMessage() *WSJSONMessageServer {
	return &WSJSONMessageServer{}
}

func (server *WSJSONMessageServer) Write(ctx context.Context, call WSJSONMessage_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSJSONMessageServer) Done(ctx context.Context, call WSJSONMessage_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wsjsonmessage: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wsjsonmessage: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
