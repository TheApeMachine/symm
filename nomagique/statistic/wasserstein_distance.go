package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
WassersteinDistanceServer calculates first Wasserstein metric (Earth Mover's Distance) between two distributions.
*/
type WassersteinDistanceServer struct {
	*runtime.System
	result float64
}

func NewWassersteinDistance(ctx context.Context) *WassersteinDistanceServer {
	server := &WassersteinDistanceServer{
		System: runtime.NewSystem(ctx, "statistic.wasserstein_distance"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *WassersteinDistanceServer) Write(ctx context.Context, call WassersteinDistance_write) error {
	pList, err := call.Args().P()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read p", err))
	}

	qList, err := call.Args().Q()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read q", err))
	}

	lenP := pList.Len()
	lenQ := qList.Len()
	sliceP := make([]float64, lenP)
	sliceQ := make([]float64, lenQ)

	for index := 0; index < lenP; index++ {
		sliceP[index] = pList.At(index)
	}

	for index := 0; index < lenQ; index++ {
		sliceQ[index] = qList.At(index)
	}

	server.result = stat.WassersteinDistance(sliceP, sliceQ, nil, nil)
	return nil
}

/*
Done returns calculated results.
*/
func (server *WassersteinDistanceServer) Done(ctx context.Context, call WassersteinDistance_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.wasserstein_distance.Done] failed to allocate done results",
			err,
		))
	}

	results.SetWassersteinDistance(server.result)
	return nil
}
