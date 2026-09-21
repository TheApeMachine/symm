package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type LogServer struct {
	out float64
}

func (srv *LogServer) Write(ctx context.Context, call Log_write) error {
	inVal := call.Args().Value()

	if inVal <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: log of non-positive value",
			nil,
		))
	}

	srv.out = math.Log(inVal)
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

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewLog() *LogServer {
	return &LogServer{}
}
