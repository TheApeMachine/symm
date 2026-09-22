package statistic

import (
	"context"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
EntropyServer calculates Shannon entropy of a probability distribution vector.
*/
type EntropyServer struct {
	*runtime.System
	result float64
}

func NewEntropy(ctx context.Context) *EntropyServer {
	server := &EntropyServer{
		System: runtime.NewSystem(ctx, "statistic.entropy"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *EntropyServer) Write(ctx context.Context, call Entropy_write) error {
	pList, err := call.Args().P()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read p", err))
	}

	length := pList.Len()
	sliceP := make([]float64, length)

	for index := 0; index < length; index++ {
		sliceP[index] = pList.At(index)
	}

	server.result = stat.Entropy(sliceP)
	return nil
}

/*
Done returns calculated results.
*/
func (server *EntropyServer) Done(ctx context.Context, call Entropy_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.entropy.Done] failed to allocate done results",
			err,
		))
	}

	results.SetEntropy(server.result)
	return nil
}
