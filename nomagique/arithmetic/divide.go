package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type DivideServer struct {
	out float64
}

func (srv *DivideServer) Write(ctx context.Context, call Divide_write) error {
	divisor := call.Args().B()

	if divisor == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"arithmetic: division by zero",
			nil,
		))
	}

	srv.out = call.Args().A() / divisor
	return nil
}

func (srv *DivideServer) Done(ctx context.Context, call Divide_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"arithmetic: alloc divide results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewDivide() *DivideServer {
	return &DivideServer{}
}
