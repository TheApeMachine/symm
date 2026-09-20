package temporal

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type LogReturnsServer struct {
	out         float64
	previous    float64
	initialized bool
}

func (srv *LogReturnsServer) Write(ctx context.Context, call LogReturns_write) error {
	inVal := call.Args().A()

	if !srv.initialized || srv.previous <= 0 || inVal <= 0 {
		srv.previous = inVal
		srv.initialized = true
		srv.out = 0
		return nil
	}

	srv.out = math.Log(inVal / srv.previous)
	srv.previous = inVal
	return nil
}

func (srv *LogReturnsServer) Done(ctx context.Context, call LogReturns_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal: alloc log returns results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewLogReturns() *LogReturnsServer {
	return &LogReturnsServer{}
}
