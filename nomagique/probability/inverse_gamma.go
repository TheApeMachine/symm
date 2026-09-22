package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
InverseGammaServer evaluates continuous Inverse-Gamma distribution.
*/
type InverseGammaServer struct {
	*runtime.System
	prob float64
	logProb float64
	cdf float64
	quantile float64
	survival float64
	mean float64
	variance float64
	stdDev float64
	rand float64
}

func NewInverseGamma(ctx context.Context) *InverseGammaServer {
	server := &InverseGammaServer{
		System: runtime.NewSystem(ctx, "probability.inverse_gamma"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *InverseGammaServer) Write(ctx context.Context, call InverseGamma_write) error {
	args := call.Args()
	dist := distuv.InverseGamma{Alpha: args.Alpha(), Beta: args.Beta()}
	x := args.X()
	p := args.P()
	server.prob = dist.Prob(x)
	server.logProb = dist.LogProb(x)
	server.cdf = dist.CDF(x)
	server.quantile = dist.Quantile(p)
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
func (server *InverseGammaServer) Done(ctx context.Context, call InverseGamma_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.inverse_gamma.Done] failed to allocate done results",
			err,
		))
	}

	results.SetProb(server.prob)
	results.SetLogProb(server.logProb)
	results.SetCdf(server.cdf)
	results.SetQuantile(server.quantile)
	results.SetSurvival(server.survival)
	results.SetMean(server.mean)
	results.SetVariance(server.variance)
	results.SetStdDev(server.stdDev)
	results.SetRand(server.rand)
	return nil
}
