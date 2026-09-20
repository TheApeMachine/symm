package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ResidualBaselineServer struct {
	out float64
}

func (srv *ResidualBaselineServer) Write(ctx context.Context, call ResidualBaseline_write) error {
	srv.out = call.Args().In()
	return nil
}

func (srv *ResidualBaselineServer) Done(ctx context.Context, call ResidualBaseline_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc residual baseline results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewResidualBaseline() *ResidualBaselineServer {
	return &ResidualBaselineServer{}
}
