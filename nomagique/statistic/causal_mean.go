package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type CausalMeanServer struct {
	scope    string
	out      float64
	count    float64
	sum      float64
	prevMean float64
}

func (srv *CausalMeanServer) Write(ctx context.Context, call CausalMean_write) error {
	scope, err := call.Args().Scope()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.causal_mean: failed to read scope", err))
	}

	// A new series starts from nothing it has not itself observed.
	if scope != srv.scope {
		*srv = CausalMeanServer{scope: scope}
	}

	inVal := call.Args().Value()
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
