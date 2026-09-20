package geometry

import (
	"context"

	"github.com/theapemachine/errnie"
)

type IntersectionServer struct {
	out bool
}

func (srv *IntersectionServer) Write(ctx context.Context, call Intersection_write) error {
	leftStart := call.Args().LeftStart()
	leftEnd := call.Args().LeftEnd()
	rightStart := call.Args().RightStart()
	rightEnd := call.Args().RightEnd()

	srv.out = leftStart < rightEnd && rightStart < leftEnd
	return nil
}

func (srv *IntersectionServer) Done(ctx context.Context, call Intersection_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"geometry: alloc intersection results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = false
	return nil
}

func NewIntersection() *IntersectionServer {
	return &IntersectionServer{}
}
