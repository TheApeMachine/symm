package crypto

import (
	"context"
	"encoding/base64"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Base64EncodeServer struct {
	*runtime.System
	out []byte
}

func NewBase64Encode(ctx context.Context) *Base64EncodeServer {
	return &Base64EncodeServer{
		System: runtime.NewSystem(ctx, "crypto.base64encode"),
	}
}

func (server *Base64EncodeServer) Write(ctx context.Context, call Base64Encode_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.base64encode: failed to read data",
			err,
		))
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	server.out = []byte(encoded)
	return nil
}

func (server *Base64EncodeServer) Done(ctx context.Context, call Base64Encode_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto.base64encode: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto.base64encode: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
