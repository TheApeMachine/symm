package calculus

import (
	"context"

	"github.com/theapemachine/errnie"
)

type SecondDifferenceServer struct {
	out float64
}

func (srv *SecondDifferenceServer) Write(ctx context.Context, call SecondDifference_write) error {
	srv.out = (call.Args().In() - call.Args().Prev1()) - (call.Args().Prev1() - call.Args().Prev2())
	return nil
}

func (srv *SecondDifferenceServer) Done(ctx context.Context, call SecondDifference_done) error {
	res, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"calculus: alloc second_difference results failed",
			err,
		))
	}

	res.SetOut(srv.out)
	srv.out = 0
	return nil
}

func NewSecondDifference() *SecondDifferenceServer {
	return &SecondDifferenceServer{}
}
