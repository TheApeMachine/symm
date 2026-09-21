package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type NegateServer struct {
	out float64
}

func (srv *NegateServer) Write(ctx context.Context, call Negate_write) error {
	srv.out = -call.Args().Value()
	return nil
}

func (srv *NegateServer) Done(ctx context.Context, call Negate_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc negate results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewNegate() *NegateServer {
	return &NegateServer{}
}
