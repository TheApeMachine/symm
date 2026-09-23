package crypto

import (
	"context"
	"os"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* SecretServer reads one credential from the process environment per trigger. */
type SecretServer struct {
	*runtime.System
	out []byte
}

func NewSecret(ctx context.Context) *SecretServer {
	return &SecretServer{
		System: runtime.NewSystem(ctx, "crypto.secret"),
	}
}

func (server *SecretServer) Write(ctx context.Context, call Secret_write) error {
	name, err := call.Args().Name()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "crypto.secret: name", err))
	}

	if name == "" {
		return server.Error(errnie.Err(errnie.Validation, "crypto.secret: a variable name is required", nil))
	}
	value := os.Getenv(name)

	if value == "" {
		return server.Error(errnie.Err(errnie.Validation, "crypto.secret: "+name+" is not set", nil))
	}
	server.out = []byte(value)
	return nil
}

func (server *SecretServer) Done(ctx context.Context, call Secret_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "crypto.secret: alloc results failed", err))
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(errnie.Internal, "crypto.secret: set out failed", err))
	}
	server.out = nil
	return nil
}
