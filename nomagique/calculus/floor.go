package calculus

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

type FloorServer struct {
	out float64
}

func (srv *FloorServer) Write(ctx context.Context, call Floor_write) error {
	srv.out = math.Floor(call.Args().In())
	return nil
}

func (srv *FloorServer) Done(ctx context.Context, call Floor_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc floor results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewFloor() *FloorServer {
	return &FloorServer{}
}
