package crypto

import (
	"context"
	"crypto/sha256"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type SHA256Server struct {
	*runtime.System
	out []byte
}

func NewSHA256(ctx context.Context) *SHA256Server {
	return &SHA256Server{
		System: runtime.NewSystem(ctx, "crypto.sha256"),
	}
}

func (server *SHA256Server) Write(ctx context.Context, call SHA256_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.sha256: failed to read data",
			err,
		))
	}

	hash := sha256.Sum256(data)
	server.out = hash[:]
	return nil
}

func (server *SHA256Server) Done(ctx context.Context, call SHA256_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto.sha256: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto.sha256: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
