package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type MaximumServer struct {
	out float64
}

func (srv *MaximumServer) Write(ctx context.Context, call Maximum_write) error {
	srv.out = math.Max(call.Args().A(), call.Args().B())
	return nil
}

func (srv *MaximumServer) Done(ctx context.Context, call Maximum_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc maximum results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewMaximum() *MaximumServer {
	return &MaximumServer{}
}
