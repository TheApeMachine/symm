package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type LogServer struct {
	out     float64
	defined bool
}

func (srv *LogServer) Write(ctx context.Context, call Log_write) error {
	value := call.Args().Value()
	srv.defined = value > 0
	srv.out = 0

	if srv.defined {
		srv.out = math.Log(value)
	}

	return nil
}

func (srv *LogServer) Done(ctx context.Context, call Log_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc log results failed",
			err,
		))
	}

	res.SetUndefined()

	if srv.defined {
		res.SetOut(srv.out)
	}

	srv.out, srv.defined = 0, false
	return nil
}

func NewLog() *LogServer {
	return &LogServer{}
}
