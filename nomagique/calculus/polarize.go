package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type PolarizeServer struct {
	out float64
}

func (srv *PolarizeServer) Write(ctx context.Context, call Polarize_write) error {
	alpha := call.Args().A()

	if alpha < 0 {
		alpha = 0
	}

	beta := -call.Args().A()

	if beta < 0 {
		beta = 0
	}

	smoothing := call.Args().B()

	if smoothing > 0 {
		alpha = alpha / (alpha + smoothing)
		beta = beta / (beta + smoothing)
	}

	srv.out = alpha - beta
	return nil
}

func (srv *PolarizeServer) Done(ctx context.Context, call Polarize_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc polarize results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewPolarize() *PolarizeServer {
	return &PolarizeServer{}
}
