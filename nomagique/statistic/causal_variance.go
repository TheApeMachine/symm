package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type CausalVarianceServer struct {
	out     float64
	count   float64
	mean    float64
	m2      float64
	prevVar float64
}

func (srv *CausalVarianceServer) Write(ctx context.Context, call CausalVariance_write) error {
	inVal := call.Args().Value()
	ret := srv.prevVar
	srv.count++
	delta := inVal - srv.mean
	srv.mean += delta / srv.count
	delta2 := inVal - srv.mean
	srv.m2 += delta * delta2

	if srv.count > 1 {
		srv.prevVar = srv.m2 / (srv.count - 1)
	}

	result := ret
	if srv.count == 1 {
		result = 0
	}

	srv.out = result
	return nil
}

func (srv *CausalVarianceServer) Done(ctx context.Context, call CausalVariance_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc causal variance results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewCausalVariance() *CausalVarianceServer {
	return &CausalVarianceServer{}
}
