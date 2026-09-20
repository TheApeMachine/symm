package ui

import (
	"context"
	"capnproto.org/go/capnp/v3"
)

type WebRTCServerImpl struct {
	Downstream func(context.Context, capnp.Ptr) error
}

func NewWebRTCServerImpl() *WebRTCServerImpl {
	return &WebRTCServerImpl{}
}

func (s *WebRTCServerImpl) Write(ctx context.Context, call WebRTCServer_write) error {
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

func (s *WebRTCServerImpl) Done(ctx context.Context, call WebRTCServer_done) error {
	return nil
}
