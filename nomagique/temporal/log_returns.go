package temporal

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type LogReturnsServer struct {
	scope       string
	out         float64
	previous    float64
	initialized bool
}

func (srv *LogReturnsServer) Write(ctx context.Context, call LogReturns_write) error {
	scope, err := call.Args().Scope()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "temporal.log_returns: failed to read scope", err))
	}

	// A new series starts from nothing it has not itself observed.
	if scope != srv.scope {
		*srv = LogReturnsServer{scope: scope}
	}

	inVal := call.Args().Value()

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
