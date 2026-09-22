package distribution

import (
	"context"
	"gonum.org/v1/gonum/spatial/r1"
	"gonum.org/v1/gonum/stat/distmv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
UniformKullbackLeiblerServer calculates Kullback-Leibler divergence between two multivariate Uniform distributions.
*/
type UniformKullbackLeiblerServer struct {
	*runtime.System
	kl float64
}

func NewUniformKullbackLeibler(ctx context.Context) *UniformKullbackLeiblerServer {
	server := &UniformKullbackLeiblerServer{
		System: runtime.NewSystem(ctx, "distribution.uniform_kullback_leibler"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *UniformKullbackLeiblerServer) Write(ctx context.Context, call UniformKullbackLeibler_write) error {
	args := call.Args()
	dim := int(args.Dim())
	minLList, err := args.MinL()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read minL", err))
	}

	maxLList, err := args.MaxL()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read maxL", err))
	}

	minRList, err := args.MinR()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read minR", err))
	}

	maxRList, err := args.MaxR()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read maxR", err))
	}

	bndsL := make([]r1.Interval, dim)
	bndsR := make([]r1.Interval, dim)

	for index := 0; index < dim; index++ {
		bndsL[index] = r1.Interval{Min: minLList.At(index), Max: maxLList.At(index)}
		bndsR[index] = r1.Interval{Min: minRList.At(index), Max: maxRList.At(index)}
	}

	leftUni := distmv.NewUniform(bndsL, nil)
	rightUni := distmv.NewUniform(bndsR, nil)
	server.kl = distmv.KullbackLeibler{}.DistUniform(leftUni, rightUni)
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *UniformKullbackLeiblerServer) Done(ctx context.Context, call UniformKullbackLeibler_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[distribution.uniform_kullback_leibler.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKl(server.kl)
	return nil
}
