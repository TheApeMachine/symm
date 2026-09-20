package store

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

type ConstantServer struct {
	value []byte
}

func NewConstant() *ConstantServer {
	return &ConstantServer{}
}

func NewConstantServer(val []byte) *ConstantServer {
	return &ConstantServer{value: bytes.Clone(val)}
}

func (server *ConstantServer) Write(ctx context.Context, call Constant_write) error {
	inData, err := call.Args().In()
	if err == nil && len(inData) > 0 {
		server.value = bytes.Clone(inData)
	}

	return nil
}

func (server *ConstantServer) Done(ctx context.Context, call Constant_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"constant: alloc results failed",
			err,
		))
	}

	if len(server.value) > 0 {
		if err := results.SetOut(server.value); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"constant: set out failed",
				err,
			))
		}
	}

	return nil
}
