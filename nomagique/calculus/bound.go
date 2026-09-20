package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type BoundServer struct {
	out float64
}

func (srv *BoundServer) Write(ctx context.Context, call Bound_write) error {
	srv.out = math.Max(call.Args().Min(), math.Min(call.Args().Max(), call.Args().In()))
	return nil
}

func (srv *BoundServer) Done(ctx context.Context, call Bound_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc bound results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewBound() *BoundServer {
	return &BoundServer{}
}
