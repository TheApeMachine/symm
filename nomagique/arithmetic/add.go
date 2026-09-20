package arithmetic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type AddServer struct {
	out float64
}

func (srv *AddServer) Write(ctx context.Context, call Add_write) error {
	srv.out = call.Args().A() + call.Args().B()
	return nil
}

func (srv *AddServer) Done(ctx context.Context, call Add_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"arithmetic: alloc add results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewAdd() *AddServer {
	return &AddServer{}
}
