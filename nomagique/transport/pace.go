package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type PaceServer struct {
	out []byte
}

func NewPace() *PaceServer {
	return &PaceServer{}
}

func (server *PaceServer) Write(ctx context.Context, call Pace_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *PaceServer) Done(ctx context.Context, call Pace_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"pace: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"pace: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
