package distribution

import (
	"context"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DirichletKullbackLeiblerServer calculates Kullback-Leibler divergence between two multivariate Dirichlet distributions.
*/
type DirichletKullbackLeiblerServer struct {
	*runtime.System
	kl float64
}

func NewDirichletKullbackLeibler(ctx context.Context) *DirichletKullbackLeiblerServer {
	server := &DirichletKullbackLeiblerServer{
		System: runtime.NewSystem(ctx, "distribution.dirichlet_kullback_leibler"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *DirichletKullbackLeiblerServer) Write(ctx context.Context, call DirichletKullbackLeibler_write) error {
	args := call.Args()
	alphaLList, err := args.AlphaL()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read alphaL", err))
	}

	alphaRList, err := args.AlphaR()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read alphaR", err))
	}

	alphaLSlice := make([]float64, alphaLList.Len())
	for index := 0; index < alphaLList.Len(); index++ {
		alphaLSlice[index] = alphaLList.At(index)
	}

	alphaRSlice := make([]float64, alphaRList.Len())
	for index := 0; index < alphaRList.Len(); index++ {
		alphaRSlice[index] = alphaRList.At(index)
	}

	leftDir := distmv.NewDirichlet(alphaLSlice, nil)
	rightDir := distmv.NewDirichlet(alphaRSlice, nil)
	server.kl = distmv.KullbackLeibler{}.DistDirichlet(leftDir, rightDir)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *DirichletKullbackLeiblerServer) Done(ctx context.Context, call DirichletKullbackLeibler_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.dirichlet_kullback_leibler.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKl(server.kl)
	return nil
}
