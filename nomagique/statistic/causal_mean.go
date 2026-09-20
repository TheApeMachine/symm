package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type CausalMeanServer struct {
	out      float64
	count    float64
	sum      float64
	prevMean float64
}

func (srv *CausalMeanServer) Write(ctx context.Context, call CausalMean_write) error {
	inVal := call.Args().In()
	ret := srv.prevMean
	srv.count++
	srv.sum += inVal
	srv.prevMean = srv.sum / srv.count

	result := ret
	if srv.count == 1 {
		result = inVal
	}

	srv.out = result
	return nil
}

func (srv *CausalMeanServer) Done(ctx context.Context, call CausalMean_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc causal mean results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewCausalMean() *CausalMeanServer {
	return &CausalMeanServer{}
}
