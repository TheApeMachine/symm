package temporal

import (
	"context"

	"github.com/theapemachine/errnie"
)

type VelocityServer struct {
	out       float64
	prevValue float64
	prevTime  float64
	hasPrior  bool
}

func (srv *VelocityServer) Write(ctx context.Context, call Velocity_write) error {
	val := call.Args().Val()
	ts := call.Args().Ts()

	if !srv.hasPrior {
		srv.prevValue = val
		srv.prevTime = ts
		srv.hasPrior = true
		srv.out = 0
		return nil
	}

	diff := val - srv.prevValue
	elapsed := ts - srv.prevTime
	srv.prevValue = val
	srv.prevTime = ts

	if elapsed > 0 {
		srv.out = diff / elapsed
		return nil
	}

	srv.out = 0
	return nil
}

func (srv *VelocityServer) Done(ctx context.Context, call Velocity_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal: alloc velocity results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewVelocity() *VelocityServer {
	return &VelocityServer{}
}
