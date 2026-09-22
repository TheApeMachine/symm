package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BhattacharyyaServer calculates Bhattacharyya distance between two probability distributions.
*/
type BhattacharyyaServer struct {
	*runtime.System
	result float64
}

func NewBhattacharyya(ctx context.Context) *BhattacharyyaServer {
	server := &BhattacharyyaServer{
		System: runtime.NewSystem(ctx, "statistic.bhattacharyya"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *BhattacharyyaServer) Write(ctx context.Context, call Bhattacharyya_write) error {
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

	server.result = stat.Bhattacharyya(sliceP, sliceQ)
	return nil
}

/*
Done returns calculated results.
*/
func (server *BhattacharyyaServer) Done(ctx context.Context, call Bhattacharyya_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.bhattacharyya.Done] failed to allocate done results",
			err,
		))
	}

	results.SetBhattacharyya(server.result)
	return nil
}
