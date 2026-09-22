package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BivariateMomentServer calculates bivariate cross-moment of orders r and s.
*/
type BivariateMomentServer struct {
	*runtime.System
	result float64
}

func NewBivariateMoment(ctx context.Context) *BivariateMomentServer {
	server := &BivariateMomentServer{
		System: runtime.NewSystem(ctx, "statistic.bivariate_moment"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *BivariateMomentServer) Write(ctx context.Context, call BivariateMoment_write) error {
	xList, err := call.Args().X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	yList, err := call.Args().Y()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read y", err))
	}

	rVal := call.Args().OrderR()
	sVal := call.Args().OrderS()
	length := xList.Len()
	sliceX := make([]float64, length)
	sliceY := make([]float64, length)

	for index := 0; index < length; index++ {
		sliceX[index] = xList.At(index)
		sliceY[index] = yList.At(index)
	}

	server.result = stat.BivariateMoment(rVal, sVal, sliceX, sliceY, nil)
	return nil
}

/*
Done returns calculated results.
*/
func (server *BivariateMomentServer) Done(ctx context.Context, call BivariateMoment_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.bivariate_moment.Done] failed to allocate done results",
			err,
		))
	}

	results.SetMoment(server.result)
	return nil
}
