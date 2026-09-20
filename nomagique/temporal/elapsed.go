package temporal

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ElapsedServer struct {
	out         float64
	previous    int64
	initialized bool
}

func (srv *ElapsedServer) Write(ctx context.Context, call Elapsed_write) error {
	inVal := call.Args().A()

	if !srv.initialized {
		srv.previous = inVal
		srv.initialized = true
		srv.out = 0
		return nil
	}

	srv.out = float64(inVal-srv.previous) / 1e9
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
