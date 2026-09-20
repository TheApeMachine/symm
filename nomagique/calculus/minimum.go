package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type MinimumServer struct {
	out float64
}

func (srv *MinimumServer) Write(ctx context.Context, call Minimum_write) error {
	srv.out = math.Min(call.Args().A(), call.Args().B())
	return nil
}

func (srv *MinimumServer) Done(ctx context.Context, call Minimum_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc minimum results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewMinimum() *MinimumServer {
	return &MinimumServer{}
}
