package crypto

import (
	"context"
	"strconv"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type NonceServer struct {
	*runtime.System
	last int64
	out  []byte
}

func NewNonce(ctx context.Context) *NonceServer {
	return &NonceServer{
		System: runtime.NewSystem(ctx, "crypto.nonce"),
	}
}

/* Write issues the clock reading, or one past the last nonce when the clock has not moved past it. */
func (server *NonceServer) Write(ctx context.Context, call Nonce_write) error {
	next := max(time.Now().UnixNano(), server.last+1)
	server.last = next
	server.out = strconv.AppendInt(nil, next, 10)
	return nil
}

func (server *NonceServer) Done(ctx context.Context, call Nonce_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal,
			"crypto.nonce: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return server.Error(errnie.Err(
				errnie.Internal,
				"crypto.nonce: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
