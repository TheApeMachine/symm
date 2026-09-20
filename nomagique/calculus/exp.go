package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type ExpServer struct {
	out float64
}

func (srv *ExpServer) Write(ctx context.Context, call Exp_write) error {
	srv.out = math.Exp(call.Args().In())
	return nil
}

func (srv *ExpServer) Done(ctx context.Context, call Exp_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc exp results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewExp() *ExpServer {
	return &ExpServer{}
}
