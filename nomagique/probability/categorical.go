package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CategoricalServer evaluates discrete categorical distribution over k categories.
*/
type CategoricalServer struct {
	*runtime.System
	prob float64
	logProb float64
	cdf float64
	mean float64
	entropy float64
	rand float64
}

func NewCategorical(ctx context.Context) *CategoricalServer {
	server := &CategoricalServer{
		System: runtime.NewSystem(ctx, "probability.categorical"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *CategoricalServer) Write(ctx context.Context, call Categorical_write) error {
	args := call.Args()
	wList, err := args.Weights()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read weights", err))
	}

	length := wList.Len()
	slice := make([]float64, length)
	for index := 0; index < length; index++ {
		slice[index] = wList.At(index)
	}

	dist := distuv.NewCategorical(slice, nil)
	x := args.X()
	server.prob = dist.Prob(x)
	server.logProb = dist.LogProb(x)
	server.cdf = dist.CDF(x)
	server.mean = dist.Mean()
	server.entropy = dist.Entropy()
	server.rand = dist.Rand()
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *CategoricalServer) Done(ctx context.Context, call Categorical_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.categorical.Done] failed to allocate done results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetLogProb(server.logProb)
	results.SetCdf(server.cdf)
	results.SetMean(server.mean)
	results.SetEntropy(server.entropy)
	results.SetRand(server.rand)
	return nil
}
