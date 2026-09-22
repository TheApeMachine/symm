package distribution

import (
	"context"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StudentsTServer evaluates multivariate Student's t distribution with degrees of freedom nu > 2.
*/
type StudentsTServer struct {
	*runtime.System
	prob    float64
	logProb float64
	mean    []float64
	cov     []float64
	rand    []float64
}

func NewStudentsT(ctx context.Context) *StudentsTServer {
	server := &StudentsTServer{
		System: runtime.NewSystem(ctx, "distribution.students_t"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *StudentsTServer) Write(ctx context.Context, call StudentsT_write) error {
	args := call.Args()
	dim := int(args.Dim())
	nu := args.Nu()
	muList, err := args.Mu()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read mu", err))
	}

	sigmaList, err := args.Sigma()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read sigma", err))
	}

	yList, err := args.Y()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read y", err))
	}

	muSlice := make([]float64, dim)
	ySlice := make([]float64, dim)
	sigmaSlice := make([]float64, dim*dim)

	for index := 0; index < dim; index++ {
		muSlice[index] = muList.At(index)
		ySlice[index] = yList.At(index)
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

	dist, ok := distmv.NewStudentsT(muSlice, sym, nu, nil)

	if !ok {
		return errnie.Error(errnie.Err(errnie.Validation, "matrix not positive definite", nil))
	}

	server.prob = dist.Prob(ySlice)
	server.logProb = dist.LogProb(ySlice)
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
	server.rand = dist.Rand(nil)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *StudentsTServer) Done(ctx context.Context, call StudentsT_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.students_t.Done] failed to allocate done results",
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

	listCov, err := results.NewCov(int32(len(server.cov)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate cov list", err))
	}

	for index, val := range server.cov {
		listCov.Set(index, val)
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
