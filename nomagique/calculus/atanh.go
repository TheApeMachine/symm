package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type AtanhServer struct {
	out float64
}

func (srv *AtanhServer) Write(ctx context.Context, call Atanh_write) error {
	srv.out = math.Atanh(call.Args().Value())
	return nil
}

func (srv *AtanhServer) Done(ctx context.Context, call Atanh_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc atanh results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewAtanh() *AtanhServer {
	return &AtanhServer{}
}
