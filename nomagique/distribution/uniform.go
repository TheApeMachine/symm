package distribution

import (
	"context"
	"gonum.org/v1/gonum/spatial/r1"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
UniformServer evaluates multivariate continuous Uniform distribution over multidimensional bounds.
*/
type UniformServer struct {
	*runtime.System
	prob float64
	logProb float64
	entropy float64
	cdf []float64
	mean []float64
	quantile []float64
	rand []float64
}

func NewUniform(ctx context.Context) *UniformServer {
	server := &UniformServer{
		System: runtime.NewSystem(ctx, "distribution.uniform"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *UniformServer) Write(ctx context.Context, call Uniform_write) error {
	args := call.Args()
	dim := int(args.Dim())
	minList, err := args.Min()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read min", err))
	}

	maxList, err := args.Max()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read max", err))
	}

	xList, err := args.X()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read x", err))
	}

	pList, err := args.P()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read p", err))
	}

	bnds := make([]r1.Interval, dim)
	xSlice := make([]float64, dim)
	pSlice := make([]float64, dim)

	for index := 0; index < dim; index++ {
		bnds[index] = r1.Interval{Min: minList.At(index), Max: maxList.At(index)}
		xSlice[index] = xList.At(index)
		pSlice[index] = pList.At(index)
	}

	dist := distmv.NewUniform(bnds, nil)
	server.prob = dist.Prob(xSlice)
	server.logProb = dist.LogProb(xSlice)
	server.entropy = dist.Entropy()
	server.cdf = dist.CDF(nil, xSlice)
	server.mean = dist.Mean(nil)
	server.quantile = dist.Quantile(nil, pSlice)
	server.rand = dist.Rand(nil)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *UniformServer) Done(ctx context.Context, call Uniform_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.uniform.Done] failed to allocate done results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetLogProb(server.logProb)
	results.SetEntropy(server.entropy)
	listCdf, err := results.NewCdf(int32(len(server.cdf)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate cdf list", err))
	}

	for index, val := range server.cdf {
		listCdf.Set(index, val)
	}

	listMean, err := results.NewMean(int32(len(server.mean)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate mean list", err))
	}

	for index, val := range server.mean {
		listMean.Set(index, val)
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
