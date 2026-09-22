package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CrossEntropyServer calculates cross-entropy between two probability distributions.
*/
type CrossEntropyServer struct {
	*runtime.System
	result float64
}

func NewCrossEntropy(ctx context.Context) *CrossEntropyServer {
	server := &CrossEntropyServer{
		System: runtime.NewSystem(ctx, "statistic.cross_entropy"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CrossEntropyServer) Write(ctx context.Context, call CrossEntropy_write) error {
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

	server.result = stat.CrossEntropy(sliceP, sliceQ)
	return nil
}

/*
Done returns calculated results.
*/
func (server *CrossEntropyServer) Done(ctx context.Context, call CrossEntropy_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.cross_entropy.Done] failed to allocate done results",
			err,
		))
	}

	results.SetCrossEntropy(server.result)
	return nil
}
