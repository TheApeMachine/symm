package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type AbsoluteServer struct {
	out float64
}

func (srv *AbsoluteServer) Write(ctx context.Context, call Absolute_write) error {
	srv.out = math.Abs(call.Args().In())
	return nil
}

func (srv *AbsoluteServer) Done(ctx context.Context, call Absolute_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc absolute results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewAbsolute() *AbsoluteServer {
	return &AbsoluteServer{}
}
