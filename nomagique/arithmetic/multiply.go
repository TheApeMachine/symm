package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type MultiplyServer struct {
	out float64
}

func (srv *MultiplyServer) Write(ctx context.Context, call Multiply_write) error {
	srv.out = call.Args().A() * call.Args().B()
	return nil
}

func (srv *MultiplyServer) Done(ctx context.Context, call Multiply_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"arithmetic: alloc multiply results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewMultiply() *MultiplyServer {
	return &MultiplyServer{}
}
