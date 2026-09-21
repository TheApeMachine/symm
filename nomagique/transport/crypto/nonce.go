package crypto

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type NonceServer struct {
	*runtime.System
	counter int64
	out     []byte
}

func NewNonce(ctx context.Context) *NonceServer {
	return &NonceServer{
		System:  runtime.NewSystem(ctx, "crypto.nonce"),
		counter: time.Now().UnixNano(),
	}
}

func (server *NonceServer) Write(ctx context.Context, call Nonce_write) error {
	nextValue := atomic.AddInt64(&server.counter, 1)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(nextValue))
	server.out = buf
	return nil
}

func (server *NonceServer) Done(ctx context.Context, call Nonce_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto.nonce: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto.nonce: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
