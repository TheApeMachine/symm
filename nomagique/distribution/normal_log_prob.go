package distribution

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalLogProbServer directly evaluates Gaussian log-likelihood using precomputed Cholesky factor.
*/
type NormalLogProbServer struct {
	*runtime.System
	logProb float64
}

func NewNormalLogProb(ctx context.Context) *NormalLogProbServer {
	server := &NormalLogProbServer{
		System: runtime.NewSystem(ctx, "distribution.normal_log_prob"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NormalLogProbServer) Write(ctx context.Context, call NormalLogProb_write) error {
	args := call.Args()
	dim := int(args.Dim())
	xList, err := args.X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	muList, err := args.Mu()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read mu", err))
	}

	cholList, err := args.CholData()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read chol data", err))
	}

	xSlice := make([]float64, dim)
	muSlice := make([]float64, dim)
	cholSlice := make([]float64, dim*dim)

	for index := 0; index < dim; index++ {
		xSlice[index] = xList.At(index)
		muSlice[index] = muList.At(index)
	}

	for index := 0; index < dim*dim; index++ {
		cholSlice[index] = cholList.At(index)
	}

	sym := mat.NewSymDense(dim, nil)
	for r := 0; r < dim; r++ {
		for c := r; c < dim; c++ {
			sym.SetSym(r, c, cholSlice[r*dim+c])
		}
	}

	var chol mat.Cholesky
	if !chol.Factorize(sym) {
		return errnie.Error(errnie.Err(errnie.Validation, "cholesky factorize failed", nil))
	}

	server.logProb = distmv.NormalLogProb(xSlice, muSlice, &chol)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *NormalLogProbServer) Done(ctx context.Context, call NormalLogProb_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.normal_log_prob.Done] failed to allocate done results",
			err,
		))
	}

	results.SetLogProb(server.logProb)
	return nil
}
