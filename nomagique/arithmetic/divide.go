package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type DivideServer struct {
	out     float64
	defined bool
}

func (srv *DivideServer) Write(ctx context.Context, call Divide_write) error {
	srv.defined = call.Args().B() != 0
	srv.out = 0

	if srv.defined {
		srv.out = call.Args().A() / call.Args().B()
	}

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

	res.SetUndefined()

	if srv.defined {
		res.SetOut(srv.out)
	}

	srv.out, srv.defined = 0, false
	return nil
}

func NewDivide() *DivideServer {
	return &DivideServer{}
}
