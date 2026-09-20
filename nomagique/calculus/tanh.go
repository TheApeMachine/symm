package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type TanhServer struct {
	out float64
}

func (srv *TanhServer) Write(ctx context.Context, call Tanh_write) error {
	srv.out = math.Tanh(call.Args().In())
	return nil
}

func (srv *TanhServer) Done(ctx context.Context, call Tanh_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc tanh results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewTanh() *TanhServer {
	return &TanhServer{}
}
