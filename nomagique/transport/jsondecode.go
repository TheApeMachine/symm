package transport

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type JSONDecodeServer struct {
	out []byte
}

func NewJSONDecode() *JSONDecodeServer {
	return &JSONDecodeServer{}
}

func (server *JSONDecodeServer) Write(ctx context.Context, call JSONDecode_write) error {
	data, _ := call.Args().Data()
	server.out = bytes.Clone(data)
	return nil
}

func (server *JSONDecodeServer) Done(ctx context.Context, call JSONDecode_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"jsondecode: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"jsondecode: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
