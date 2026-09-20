package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type BearerAuthServer struct {
	out []byte
}

func NewBearerAuth() *BearerAuthServer {
	return &BearerAuthServer{}
}

func (server *BearerAuthServer) Write(ctx context.Context, call BearerAuth_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *BearerAuthServer) Done(ctx context.Context, call BearerAuth_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"bearerauth: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"bearerauth: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
