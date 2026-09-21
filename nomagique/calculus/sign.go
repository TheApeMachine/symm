package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SignServer struct {
	out float64
}

func (srv *SignServer) Write(ctx context.Context, call Sign_write) error {
	inVal := call.Args().Value()

	if inVal < 0 {
		srv.out = -1
		return nil
	}

	if inVal > 0 {
		srv.out = 1
		return nil
	}

	srv.out = 0
	return nil
}

func (srv *SignServer) Done(ctx context.Context, call Sign_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc sign results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewSign() *SignServer {
	return &SignServer{}
}
