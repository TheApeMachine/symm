package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NoncentralTServer evaluates continuous Noncentral Student's t distribution.
*/
type NoncentralTServer struct {
	*runtime.System
	prob float64
	logProb float64
	cdf float64
	quantile float64
	mean float64
	variance float64
}

func NewNoncentralT(ctx context.Context) *NoncentralTServer {
	server := &NoncentralTServer{
		System: runtime.NewSystem(ctx, "probability.noncentral_t"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NoncentralTServer) Write(ctx context.Context, call NoncentralT_write) error {
	args := call.Args()
	dist := distuv.NoncentralT{Nu: args.Nu(), Mu: args.Mu()}
	x := args.X()
	p := args.P()
	server.prob = dist.Prob(x)
	server.logProb = dist.LogProb(x)
	server.cdf = dist.CDF(x)
	server.quantile = dist.Quantile(p)
	server.mean = dist.Mean()
	server.variance = dist.Variance()
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *NoncentralTServer) Done(ctx context.Context, call NoncentralT_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.noncentral_t.Done] failed to allocate done results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetLogProb(server.logProb)
	results.SetCdf(server.cdf)
	results.SetQuantile(server.quantile)
	results.SetMean(server.mean)
	results.SetVariance(server.variance)
	return nil
}
