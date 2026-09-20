package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type JSONEncodeServer struct {
	out []byte
}

func NewJSONEncode() *JSONEncodeServer {
	return &JSONEncodeServer{}
}

func (server *JSONEncodeServer) Write(ctx context.Context, call JSONEncode_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *JSONEncodeServer) Done(ctx context.Context, call JSONEncode_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"jsonencode: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"jsonencode: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
