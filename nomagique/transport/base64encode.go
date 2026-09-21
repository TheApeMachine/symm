package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type Base64EncodeServer struct {
	out []byte
}

func NewBase64Encode() *Base64EncodeServer {
	return &Base64EncodeServer{}
}

func (server *Base64EncodeServer) Write(ctx context.Context, call Base64Encode_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *Base64EncodeServer) Done(ctx context.Context, call Base64Encode_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"base64encode: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"base64encode: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
