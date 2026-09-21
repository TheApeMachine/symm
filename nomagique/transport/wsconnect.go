package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WSConnectServer struct {
	out []byte
}

func NewWSConnect() *WSConnectServer {
	return &WSConnectServer{}
}

func (server *WSConnectServer) Write(ctx context.Context, call WSConnect_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WSConnectServer) Done(ctx context.Context, call WSConnect_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"wsconnect: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"wsconnect: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
