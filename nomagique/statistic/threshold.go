package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ThresholdServer struct {
	out   float64
	Band  float64
	Rest  float64
	Lower float64
	Upper float64
}

func (srv *ThresholdServer) Write(ctx context.Context, call Threshold_write) error {
	inVal := call.Args().In()
	result := srv.Rest

	if inVal < srv.Band {
		result = srv.Upper
	}

	if inVal > 1.0-srv.Band {
		result = srv.Lower
	}

	srv.out = result
	return nil
}

func (srv *ThresholdServer) Done(ctx context.Context, call Threshold_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"statistic: alloc threshold results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewThreshold(band, rest, lower, upper float64) *ThresholdServer {
	return &ThresholdServer{
		Band:  band,
		Rest:  rest,
		Lower: lower,
		Upper: upper,
	}
}
