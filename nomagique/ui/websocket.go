package ui

import (
	"context"
	"capnproto.org/go/capnp/v3"
)

type WebSocketServerImpl struct {
	Downstream func(context.Context, capnp.Ptr) error
}

func NewWebSocketServerImpl() *WebSocketServerImpl {
	return &WebSocketServerImpl{}
}

func (s *WebSocketServerImpl) Write(ctx context.Context, call WebSocketServer_write) error {
	args, err := call.Args().Server()
	if err != nil {
		// fallback to see if it's named something else
		return err
	}
	
	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}
	
	if s.Downstream != nil {
		return s.Downstream(ctx, payloadPtr)
	}
	return nil
}

func (s *WebSocketServerImpl) Done(ctx context.Context, call WebSocketServer_done) error {
	return nil
}
