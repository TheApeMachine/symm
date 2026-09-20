package ui

import (
	"context"
	"capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/types"
)

type WebSocketServerNode types.StreamNode[any, any]

type WebSocketServerImpl struct {
	Downstream func(context.Context, capnp.Ptr) error
}

func NewWebSocketServer() WebSocketServerNode {
	server := &WebSocketServerImpl{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {
			server.Downstream = func(c context.Context, ptr capnp.Ptr) error {
				return next(c, ptr)
			}
		},
	)
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
