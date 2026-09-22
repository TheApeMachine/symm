package distribution

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalKullbackLeiblerServer calculates Kullback-Leibler divergence between two multivariate Normal distributions.
*/
type NormalKullbackLeiblerServer struct {
	*runtime.System
	kl float64
}

func NewNormalKullbackLeibler(ctx context.Context) *NormalKullbackLeiblerServer {
	server := &NormalKullbackLeiblerServer{
		System: runtime.NewSystem(ctx, "distribution.normal_kullback_leibler"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NormalKullbackLeiblerServer) Write(ctx context.Context, call NormalKullbackLeibler_write) error {
	args := call.Args()
	dim := int(args.Dim())
	muLList, err := args.MuL()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read muL", err))
	}

	sigmaLList, err := args.SigmaL()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read sigmaL", err))
	}

	muRList, err := args.MuR()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read muR", err))
	}

	sigmaRList, err := args.SigmaR()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read sigmaR", err))
	}

	muLSlice := make([]float64, dim)
	muRSlice := make([]float64, dim)
	sigmaLSlice := make([]float64, dim*dim)
	sigmaRSlice := make([]float64, dim*dim)

	for index := 0; index < dim; index++ {
		muLSlice[index] = muLList.At(index)
		muRSlice[index] = muRList.At(index)
	}

	for index := 0; index < dim*dim; index++ {
		sigmaLSlice[index] = sigmaLList.At(index)
		sigmaRSlice[index] = sigmaRList.At(index)
	}

	symL := mat.NewSymDense(dim, nil)
	symR := mat.NewSymDense(dim, nil)
	for r := 0; r < dim; r++ {
		for c := r; c < dim; c++ {
			symL.SetSym(r, c, sigmaLSlice[r*dim+c])
			symR.SetSym(r, c, sigmaRSlice[r*dim+c])
		}
	}

	leftNorm, ok := distmv.NewNormal(muLSlice, symL, nil)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Validation, "left sigma not positive definite", nil))
	}

	rightNorm, ok := distmv.NewNormal(muRSlice, symR, nil)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Validation, "right sigma not positive definite", nil))
	}

	server.kl = distmv.KullbackLeibler{}.DistNormal(leftNorm, rightNorm)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *NormalKullbackLeiblerServer) Done(ctx context.Context, call NormalKullbackLeibler_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.normal_kullback_leibler.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKl(server.kl)
	return nil
}
