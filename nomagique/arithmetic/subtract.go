package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SubtractServer struct {
	out float64
}

func (srv *SubtractServer) Write(ctx context.Context, call Subtract_write) error {
	srv.out = call.Args().A() - call.Args().B()
	return nil
}

func (srv *SubtractServer) Done(ctx context.Context, call Subtract_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"arithmetic: alloc subtract results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewSubtract() *SubtractServer {
	return &SubtractServer{}
}
