package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type HMACSHA256Server struct {
	out []byte
}

func NewHMACSHA256() *HMACSHA256Server {
	return &HMACSHA256Server{}
}

func (server *HMACSHA256Server) Write(ctx context.Context, call HMACSHA256_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *HMACSHA256Server) Done(ctx context.Context, call HMACSHA256_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hmacsha256: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"hmacsha256: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
