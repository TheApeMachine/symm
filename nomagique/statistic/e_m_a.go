package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type EMAServer struct {
	out         float64
	ema         float64
	initialized bool
	Alpha       float64
}

func (srv *EMAServer) Write(ctx context.Context, call EMA_write) error {
	inVal := call.Args().Value()

	if srv.Alpha <= 0 {
		srv.out = inVal
		return nil
	}

	if !srv.initialized {
		srv.ema = inVal
		srv.initialized = true
		srv.out = inVal
		return nil
	}

	srv.ema = srv.Alpha*inVal + (1-srv.Alpha)*srv.ema
	srv.out = srv.ema
	return nil
}

func (srv *EMAServer) Done(ctx context.Context, call EMA_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc ema results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewEMA(alpha ...float64) *EMAServer {
	val := 0.1
	if len(alpha) > 0 {
		val = alpha[0]
	}

	return &EMAServer{Alpha: val}
}
