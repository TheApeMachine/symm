package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type Base64DecodeServer struct {
	out []byte
}

func NewBase64Decode() *Base64DecodeServer {
	return &Base64DecodeServer{}
}

func (server *Base64DecodeServer) Write(ctx context.Context, call Base64Decode_write) error {
	data, _ := call.Args().In()
	server.out = bytes.Clone(data)
	return nil
}

func (server *Base64DecodeServer) Done(ctx context.Context, call Base64Decode_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"base64decode: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"base64decode: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
