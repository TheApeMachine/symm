package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type HMACSHA512Server struct {
	out []byte
}

func NewHMACSHA512() *HMACSHA512Server {
	return &HMACSHA512Server{}
}

func (server *HMACSHA512Server) Write(ctx context.Context, call HMACSHA512_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *HMACSHA512Server) Done(ctx context.Context, call HMACSHA512_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hmacsha512: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"hmacsha512: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
