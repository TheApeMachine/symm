package distribution

import (
	"context"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DirichletServer evaluates multivariate Dirichlet distribution on the standard simplex.
*/
type DirichletServer struct {
	*runtime.System
	prob float64
	logProb float64
	mean []float64
	rand []float64
}

func NewDirichlet(ctx context.Context) *DirichletServer {
	server := &DirichletServer{
		System: runtime.NewSystem(ctx, "distribution.dirichlet"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *DirichletServer) Write(ctx context.Context, call Dirichlet_write) error {
	args := call.Args()
	alphaList, err := args.Alpha()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read alpha", err))
	}

	xList, err := args.X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	alphaSlice := make([]float64, alphaList.Len())
	for index := 0; index < alphaList.Len(); index++ {
		alphaSlice[index] = alphaList.At(index)
	}

	xSlice := make([]float64, xList.Len())
	for index := 0; index < xList.Len(); index++ {
		xSlice[index] = xList.At(index)
	}

	dist := distmv.NewDirichlet(alphaSlice, nil)
	server.prob = dist.Prob(xSlice)
	server.logProb = dist.LogProb(xSlice)
	server.mean = dist.Mean(nil)
	server.rand = dist.Rand(nil)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *DirichletServer) Done(ctx context.Context, call Dirichlet_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.dirichlet.Done] failed to allocate done results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetLogProb(server.logProb)
	listMean, err := results.NewMean(int32(len(server.mean)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate mean list", err))
	}

	for index, val := range server.mean {
		listMean.Set(index, val)
	}

	listRand, err := results.NewRand(int32(len(server.rand)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate rand list", err))
	}

	for index, val := range server.rand {
		listRand.Set(index, val)
	}

	return nil
}
