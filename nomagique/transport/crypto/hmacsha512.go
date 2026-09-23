package crypto

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type HMACSHA512Server struct {
	*runtime.System
	out []byte
}

func NewHMACSHA512(ctx context.Context) *HMACSHA512Server {
	return &HMACSHA512Server{
		System: runtime.NewSystem(ctx, "crypto.hmacsha512"),
	}
}

func (server *HMACSHA512Server) Write(ctx context.Context, call HMACSHA512_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.hmacsha512: failed to read data",
			err,
		))
	}
	key, err := call.Args().Key()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.BadRequest,
			"crypto.hmacsha512: failed to read key",
			err,
		))
	}

	if len(key) == 0 {
		return server.Error(errnie.Err(
			errnie.Validation,
			"crypto.hmacsha512: a key is required",
			nil,
		))
	}
	mac := hmac.New(sha512.New, key)
	mac.Write(data)
	server.out = mac.Sum(nil)
	return nil
}

func (server *HMACSHA512Server) Done(ctx context.Context, call HMACSHA512_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal,
			"crypto.hmacsha512: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return server.Error(errnie.Err(
				errnie.Internal,
				"crypto.hmacsha512: set out failed",
				err,
			))
		}
	}

	server.out = nil
	return nil
}
