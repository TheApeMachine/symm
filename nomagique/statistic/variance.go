package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type VarianceServer struct {
	out   float64
	count float64
	mean  float64
	m2    float64
}

func (srv *VarianceServer) Write(ctx context.Context, call Variance_write) error {
	inVal := call.Args().In()
	srv.count++
	delta := inVal - srv.mean
	srv.mean += delta / srv.count
	delta2 := inVal - srv.mean
	srv.m2 += delta * delta2

	result := float64(0)
	if srv.count > 1 {
		result = srv.m2 / (srv.count - 1)
	}

	srv.out = result
	return nil
}

func (srv *VarianceServer) Done(ctx context.Context, call Variance_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc variance results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewVariance() *VarianceServer {
	return &VarianceServer{}
}
