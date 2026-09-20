package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type NonceServer struct {
	out []byte
}

func NewNonce() *NonceServer {
	return &NonceServer{}
}

func (server *NonceServer) Write(ctx context.Context, call Nonce_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *NonceServer) Done(ctx context.Context, call Nonce_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"nonce: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"nonce: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
