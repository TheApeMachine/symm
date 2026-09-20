package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SquareServer struct {
	out float64
}

func (srv *SquareServer) Write(ctx context.Context, call Square_write) error {
	inVal := call.Args().In()
	srv.out = inVal * inVal
	return nil
}

func (srv *SquareServer) Done(ctx context.Context, call Square_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc square results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewSquare() *SquareServer {
	return &SquareServer{}
}
