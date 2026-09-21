package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type MeanServer struct {
	out   float64
	count float64
	sum   float64
}

func (srv *MeanServer) Write(ctx context.Context, call Mean_write) error {
	inVal := call.Args().Value()
	srv.count++
	srv.sum += inVal
	srv.out = srv.sum / srv.count
	return nil
}

func (srv *MeanServer) Done(ctx context.Context, call Mean_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc mean results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewMean() *MeanServer {
	return &MeanServer{}
}
