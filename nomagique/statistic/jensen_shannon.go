package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
JensenShannonServer calculates symmetric Jensen-Shannon divergence between two distributions.
*/
type JensenShannonServer struct {
	*runtime.System
	result float64
}

func NewJensenShannon(ctx context.Context) *JensenShannonServer {
	server := &JensenShannonServer{
		System: runtime.NewSystem(ctx, "statistic.jensen_shannon"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *JensenShannonServer) Write(ctx context.Context, call JensenShannon_write) error {
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

	server.result = stat.JensenShannon(sliceP, sliceQ)
	return nil
}

/*
Done returns calculated results.
*/
func (server *JensenShannonServer) Done(ctx context.Context, call JensenShannon_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.jensen_shannon.Done] failed to allocate done results",
			err,
		))
	}

	results.SetJensenShannon(server.result)
	return nil
}
