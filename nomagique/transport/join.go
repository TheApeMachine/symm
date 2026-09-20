package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type JoinServer struct {
	out []byte
}

func NewJoin() *JoinServer {
	return &JoinServer{}
}

func (server *JoinServer) Write(ctx context.Context, call Join_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *JoinServer) Done(ctx context.Context, call Join_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"join: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"join: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
