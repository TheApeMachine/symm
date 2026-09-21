package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type RelativeChangeServer struct {
	out float64
}

func (srv *RelativeChangeServer) Write(ctx context.Context, call RelativeChange_write) error {
	prev := call.Args().Prev()

	if prev == 0 {
		srv.out = 0
		return nil
	}

	srv.out = (call.Args().Value() - prev) / prev
	return nil
}

func (srv *RelativeChangeServer) Done(ctx context.Context, call RelativeChange_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc relative_change results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewRelativeChange() *RelativeChangeServer {
	return &RelativeChangeServer{}
}
