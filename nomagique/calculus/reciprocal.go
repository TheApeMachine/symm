package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type ReciprocalServer struct {
	out     float64
	defined bool
}

func (srv *ReciprocalServer) Write(ctx context.Context, call Reciprocal_write) error {
	value := call.Args().Value()
	srv.defined = value != 0
	srv.out = 0

	if srv.defined {
		srv.out = 1 / value
	}

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

	res.SetUndefined()

	if srv.defined {
		res.SetOut(srv.out)
	}

	srv.out, srv.defined = 0, false
	return nil
}

func NewReciprocal() *ReciprocalServer {
	return &ReciprocalServer{}
}
