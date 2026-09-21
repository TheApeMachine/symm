package crypto

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type HMACSHA256Server struct {
	*runtime.System
	out []byte
}

func NewHMACSHA256(ctx context.Context) *HMACSHA256Server {
	return &HMACSHA256Server{
		System: runtime.NewSystem(ctx, "crypto.hmacsha256"),
	}
}

func (server *HMACSHA256Server) Write(ctx context.Context, call HMACSHA256_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.hmacsha256: failed to read data",
			err,
		))
	}

	mac := hmac.New(sha256.New, nil)
	mac.Write(data)
	server.out = mac.Sum(nil)
	return nil
}

func (server *HMACSHA256Server) Done(ctx context.Context, call HMACSHA256_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"crypto.hmacsha256: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"crypto.hmacsha256: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
