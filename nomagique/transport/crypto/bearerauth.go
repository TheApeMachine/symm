package crypto

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type BearerAuthServer struct {
	*runtime.System
	out []byte
}

func NewBearerAuth(ctx context.Context) *BearerAuthServer {
	return &BearerAuthServer{
		System: runtime.NewSystem(ctx, "crypto.bearerauth"),
	}
}

func (server *BearerAuthServer) Write(ctx context.Context, call BearerAuth_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.bearerauth: failed to read data",
			err,
		))
	}

	server.out = append([]byte("Bearer "), data...)
	return nil
}

func (server *BearerAuthServer) Done(ctx context.Context, call BearerAuth_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto.bearerauth: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto.bearerauth: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
