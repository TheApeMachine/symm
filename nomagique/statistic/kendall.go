package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KendallServer calculates Kendall rank correlation tau between two sample vectors.
*/
type KendallServer struct {
	*runtime.System
	result float64
}

func NewKendall(ctx context.Context) *KendallServer {
	server := &KendallServer{
		System: runtime.NewSystem(ctx, "statistic.kendall"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KendallServer) Write(ctx context.Context, call Kendall_write) error {
	xList, err := call.Args().X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	yList, err := call.Args().Y()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read y", err))
	}

	length := xList.Len()
	sliceX := make([]float64, length)
	sliceY := make([]float64, length)

	for index := 0; index < length; index++ {
		sliceX[index] = xList.At(index)
		sliceY[index] = yList.At(index)
	}

	server.result = stat.Kendall(sliceX, sliceY, nil)
	return nil
}

/*
Done returns calculated results.
*/
func (server *KendallServer) Done(ctx context.Context, call Kendall_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.kendall.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKendall(server.result)
	return nil
}
