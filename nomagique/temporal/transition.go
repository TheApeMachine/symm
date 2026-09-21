package temporal

import (
	"context"

	"github.com/theapemachine/errnie"
)

type TransitionServer struct {
	out []byte
}

func (srv *TransitionServer) Write(ctx context.Context, call Transition_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"temporal: read transition arg failed",
			err,
		))
	}

	srv.out = data
	return nil
}

func (srv *TransitionServer) Done(ctx context.Context, call Transition_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal: alloc transition results failed",
			err,
		))
	}

	if err := res.SetOut(srv.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal: set transition out failed",
			err,
		))
	}

	srv.out = nil
	return nil
}

func NewTransition() *TransitionServer {
	return &TransitionServer{}
}
