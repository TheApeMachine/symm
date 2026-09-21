package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type SqrtServer struct {
	out float64
}

func (srv *SqrtServer) Write(ctx context.Context, call Sqrt_write) error {
	inVal := call.Args().Value()

	if inVal < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: sqrt of negative value",
			nil,
		))
	}

	srv.out = math.Sqrt(inVal)
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

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewSqrt() *SqrtServer {
	return &SqrtServer{}
}
