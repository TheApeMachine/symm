package crypto

import (
	"context"
	"encoding/base64"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Base64DecodeServer struct {
	*runtime.System
	out []byte
}

func NewBase64Decode(ctx context.Context) *Base64DecodeServer {
	return &Base64DecodeServer{
		System: runtime.NewSystem(ctx, "crypto.base64decode"),
	}
}

func (server *Base64DecodeServer) Write(ctx context.Context, call Base64Decode_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.base64decode: failed to read data",
			err,
		))
	}

	decoded, decodeErr := base64.StdEncoding.DecodeString(string(data))

	if decodeErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto.base64decode: failed to decode base64 string",
			decodeErr,
		))
	}

	server.out = decoded
	return nil
}

func (server *Base64DecodeServer) Done(ctx context.Context, call Base64Decode_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto.base64decode: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto.base64decode: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
