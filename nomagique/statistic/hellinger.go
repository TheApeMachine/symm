package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
HellingerServer calculates Hellinger distance between two probability distributions.
*/
type HellingerServer struct {
	*runtime.System
	result float64
}

func NewHellinger(ctx context.Context) *HellingerServer {
	server := &HellingerServer{
		System: runtime.NewSystem(ctx, "statistic.hellinger"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *HellingerServer) Write(ctx context.Context, call Hellinger_write) error {
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

	server.result = stat.Hellinger(sliceP, sliceQ)
	return nil
}

/*
Done returns calculated results.
*/
func (server *HellingerServer) Done(ctx context.Context, call Hellinger_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.hellinger.Done] failed to allocate done results",
			err,
		))
	}

	results.SetHellinger(server.result)
	return nil
}
