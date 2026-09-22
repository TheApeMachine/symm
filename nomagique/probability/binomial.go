package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
BinomialServer evaluates discrete Binomial distribution for n independent Bernoulli trials.
*/
type BinomialServer struct {
	*runtime.System
	prob     float64
	logProb  float64
	cdf      float64
	survival float64
	mean     float64
	variance float64
	stdDev   float64
	rand     float64
}

func NewBinomial(ctx context.Context) *BinomialServer {
	server := &BinomialServer{
		System: runtime.NewSystem(ctx, "probability.binomial"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *BinomialServer) Write(ctx context.Context, call Binomial_write) error {
	args := call.Args()
	dist := distuv.Binomial{N: args.N(), P: args.P()}
	x := args.X()
	server.prob = dist.Prob(x)
	server.logProb = dist.LogProb(x)
	server.cdf = dist.CDF(x)
	server.survival = dist.Survival(x)
	server.mean = dist.Mean()
	server.variance = dist.Variance()
	server.stdDev = dist.StdDev()
	server.rand = dist.Rand()
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *BinomialServer) Done(ctx context.Context, call Binomial_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.binomial.Done] failed to allocate done results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetLogProb(server.logProb)
	results.SetCdf(server.cdf)
	results.SetSurvival(server.survival)
	results.SetMean(server.mean)
	results.SetVariance(server.variance)
	results.SetStdDev(server.stdDev)
	results.SetRand(server.rand)
	return nil
}
