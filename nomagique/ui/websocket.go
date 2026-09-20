package ui

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WebSocketServerImpl struct {
	out []byte
}

func NewWebSocketServer() *WebSocketServerImpl {
	return &WebSocketServerImpl{}
}

func NewWebSocketServerImpl() *WebSocketServerImpl {
	return &WebSocketServerImpl{}
}

func (server *WebSocketServerImpl) Write(ctx context.Context, call WebSocketServer_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WebSocketServerImpl) Done(ctx context.Context, call WebSocketServer_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"websocket: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"websocket: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
