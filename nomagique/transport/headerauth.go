package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type HeaderAuthServer struct {
	out []byte
}

func NewHeaderAuth() *HeaderAuthServer {
	return &HeaderAuthServer{}
}

func (server *HeaderAuthServer) Write(ctx context.Context, call HeaderAuth_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *HeaderAuthServer) Done(ctx context.Context, call HeaderAuth_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"headerauth: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"headerauth: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
