package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KullbackLeiblerServer calculates Kullback-Leibler divergence between two distributions.
*/
type KullbackLeiblerServer struct {
	*runtime.System
	result float64
}

func NewKullbackLeibler(ctx context.Context) *KullbackLeiblerServer {
	server := &KullbackLeiblerServer{
		System: runtime.NewSystem(ctx, "statistic.kullback_leibler"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KullbackLeiblerServer) Write(ctx context.Context, call KullbackLeibler_write) error {
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

	server.result = stat.KullbackLeibler(sliceP, sliceQ)
	return nil
}

/*
Done returns calculated results.
*/
func (server *KullbackLeiblerServer) Done(ctx context.Context, call KullbackLeibler_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.kullback_leibler.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKullbackLeibler(server.result)
	return nil
}
