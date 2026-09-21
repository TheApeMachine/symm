package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ResidualDivergenceServer struct {
	out float64
}

func (srv *ResidualDivergenceServer) Write(ctx context.Context, call ResidualDivergence_write) error {
	srv.out = call.Args().Value()
	return nil
}

func (srv *ResidualDivergenceServer) Done(ctx context.Context, call ResidualDivergence_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc residual divergence results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewResidualDivergence() *ResidualDivergenceServer {
	return &ResidualDivergenceServer{}
}
