package temporal

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ElapsedServer struct {
	scope       string
	out         float64
	previous    float64
	initialized bool
}

func (srv *ElapsedServer) Write(ctx context.Context, call Elapsed_write) error {
	scope, err := call.Args().Scope()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "temporal.elapsed: failed to read scope", err))
	}

	// A new series starts from nothing it has not itself observed.
	if scope != srv.scope {
		*srv = ElapsedServer{scope: scope}
	}

	inVal := call.Args().Timestamp()

	if !srv.initialized {
		srv.previous = inVal
		srv.initialized = true
		srv.out = 0
		return nil
	}

	srv.out = (inVal - srv.previous) / 1e9

	if call.Args().Origin() {
		return nil
	}

	srv.previous = inVal
	return nil
}

func (srv *ElapsedServer) Done(ctx context.Context, call Elapsed_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal: alloc elapsed results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewElapsed() *ElapsedServer {
	return &ElapsedServer{}
}
