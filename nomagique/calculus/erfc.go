package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type ErfcServer struct {
	out float64
}

func (srv *ErfcServer) Write(ctx context.Context, call Erfc_write) error {
	srv.out = math.Erfc(call.Args().In())
	return nil
}

func (srv *ErfcServer) Done(ctx context.Context, call Erfc_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc erfc results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewErfc() *ErfcServer {
	return &ErfcServer{}
}
