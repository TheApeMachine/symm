package store

import (
	"context"

	"github.com/theapemachine/errnie"
)

type KeyServer struct {
	val   float64
	found bool
}

func NewKey() *KeyServer {
	return &KeyServer{}
}

func (server *KeyServer) Write(ctx context.Context, call Key_write) error {
	server.val = 0.0
	server.found = false
	return nil
}

func (server *KeyServer) Done(ctx context.Context, call Key_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"key: alloc results failed",
			err,
		))
	}

	results.SetValue(server.val)
	results.SetFound(server.found)
	server.val = 0
	server.found = false
	return nil
}
