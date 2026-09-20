package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type TimestampServer struct {
	out []byte
}

func NewTimestamp() *TimestampServer {
	return &TimestampServer{}
}

func (server *TimestampServer) Write(ctx context.Context, call Timestamp_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *TimestampServer) Done(ctx context.Context, call Timestamp_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"timestamp: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"timestamp: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
