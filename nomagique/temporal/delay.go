package temporal

import (
	"context"

	"github.com/theapemachine/errnie"
)

type DelayServer struct {
	out     float64
	horizon int
	buffer  []float64
	idx     int
}

func (srv *DelayServer) Write(ctx context.Context, call Delay_write) error {
	inVal := call.Args().In()
	if srv.horizon <= 0 {
		srv.horizon = 1
	}

	if len(srv.buffer) < srv.horizon {
		srv.buffer = append(srv.buffer, inVal)
		srv.out = inVal
		return nil
	}

	delayed := srv.buffer[srv.idx]
	srv.buffer[srv.idx] = inVal
	srv.idx = (srv.idx + 1) % srv.horizon
	srv.out = delayed
	return nil
}

func (srv *DelayServer) Done(ctx context.Context, call Delay_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal: alloc delay results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewDelay() *DelayServer {
	return &DelayServer{horizon: 1}
}
