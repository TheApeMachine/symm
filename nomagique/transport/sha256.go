package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type SHA256Server struct {
	out []byte
}

func NewSHA256() *SHA256Server {
	return &SHA256Server{}
}

func (server *SHA256Server) Write(ctx context.Context, call SHA256_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *SHA256Server) Done(ctx context.Context, call SHA256_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"sha256: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"sha256: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
