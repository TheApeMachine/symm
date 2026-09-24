package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type SqrtServer struct {
	out     float64
	defined bool
}

func (srv *SqrtServer) Write(ctx context.Context, call Sqrt_write) error {
	value := call.Args().Value()
	srv.defined = value >= 0
	srv.out = 0

	if srv.defined {
		srv.out = math.Sqrt(value)
	}

	return nil
}

func (srv *SqrtServer) Done(ctx context.Context, call Sqrt_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc sqrt results failed",
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

func NewSqrt() *SqrtServer {
	return &SqrtServer{}
}
