package crypto

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type HeaderAuthServer struct {
	*runtime.System
	out []byte
}

func NewHeaderAuth(ctx context.Context) *HeaderAuthServer {
	return &HeaderAuthServer{
		System: runtime.NewSystem(ctx, "crypto.headerauth"),
	}
}

func (server *HeaderAuthServer) Write(ctx context.Context, call HeaderAuth_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.headerauth: failed to read data",
			err,
		))
	}

	server.out = append([]byte("Header: "), data...)
	return nil
}

func (server *HeaderAuthServer) Done(ctx context.Context, call HeaderAuth_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto.headerauth: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto.headerauth: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
