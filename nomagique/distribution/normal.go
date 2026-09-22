package distribution

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalServer evaluates multivariate Normal Gaussian distribution.
*/
type NormalServer struct {
	*runtime.System
	prob float64
	logProb float64
	entropy float64
	mean []float64
	cov []float64
	quantile []float64
	rand []float64
}

func NewNormal(ctx context.Context) *NormalServer {
	server := &NormalServer{
		System: runtime.NewSystem(ctx, "distribution.normal"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NormalServer) Write(ctx context.Context, call Normal_write) error {
	args := call.Args()
	dim := int(args.Dim())
	muList, err := args.Mu()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read mu", err))
	}

	sigmaList, err := args.Sigma()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read sigma", err))
	}

	xList, err := args.X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	pList, err := args.P()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read p", err))
	}

	muSlice := make([]float64, dim)
	xSlice := make([]float64, dim)
	pSlice := make([]float64, dim)
	sigmaSlice := make([]float64, dim*dim)

	for index := 0; index < dim; index++ {
		muSlice[index] = muList.At(index)
		xSlice[index] = xList.At(index)
		pSlice[index] = pList.At(index)
	}

	for index := 0; index < dim*dim; index++ {
		sigmaSlice[index] = sigmaList.At(index)
	}

	sym := mat.NewSymDense(dim, nil)
	for r := 0; r < dim; r++ {
		for c := r; c < dim; c++ {
			sym.SetSym(r, c, sigmaSlice[r*dim+c])
		}
	}

	dist, ok := distmv.NewNormal(muSlice, sym, nil)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Validation, "matrix not positive definite", nil))
	}

	server.prob = dist.Prob(xSlice)
	server.logProb = dist.LogProb(xSlice)
	server.entropy = dist.Entropy()
	server.mean = dist.Mean(nil)

	covSym := mat.NewSymDense(dim, nil)
	dist.CovarianceMatrix(covSym)
	covFlat := make([]float64, dim*dim)
	for r := 0; r < dim; r++ {
		for c := 0; c < dim; c++ {
			covFlat[r*dim+c] = covSym.At(r, c)
		}
	}
	server.cov = covFlat
	server.quantile = dist.Quantile(nil, pSlice)
	server.rand = dist.Rand(nil)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *NormalServer) Done(ctx context.Context, call Normal_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.normal.Done] failed to allocate done results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetLogProb(server.logProb)
	results.SetEntropy(server.entropy)
	listMean, err := results.NewMean(int32(len(server.mean)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate mean list", err))
	}

	for index, val := range server.mean {
		listMean.Set(index, val)
	}

	listCov, err := results.NewCov(int32(len(server.cov)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate cov list", err))
	}

	for index, val := range server.cov {
		listCov.Set(index, val)
	}

	listQuantile, err := results.NewQuantile(int32(len(server.quantile)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate quantile list", err))
	}

	for index, val := range server.quantile {
		listQuantile.Set(index, val)
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
