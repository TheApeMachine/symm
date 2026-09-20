package ui

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type WebRTCServerImpl struct {
	out []byte
}

func NewWebRTCServer() *WebRTCServerImpl {
	return &WebRTCServerImpl{}
}

func NewWebRTCServerImpl() *WebRTCServerImpl {
	return &WebRTCServerImpl{}
}

func (server *WebRTCServerImpl) Write(ctx context.Context, call WebRTCServer_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *WebRTCServerImpl) Done(ctx context.Context, call WebRTCServer_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"webrtc: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"webrtc: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
