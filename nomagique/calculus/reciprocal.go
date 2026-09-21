package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ReciprocalServer struct {
	out float64
}

func (srv *ReciprocalServer) Write(ctx context.Context, call Reciprocal_write) error {
	inVal := call.Args().Value()

	if inVal == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: reciprocal of zero",
			nil,
		))
	}

	srv.out = 1.0 / inVal
	return nil
}

func (srv *ReciprocalServer) Done(ctx context.Context, call Reciprocal_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc reciprocal results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewReciprocal() *ReciprocalServer {
	return &ReciprocalServer{}
}
