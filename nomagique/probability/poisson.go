package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
PoissonServer evaluates discrete Poisson arrival distribution.
*/
type PoissonServer struct {
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

func NewPoisson(ctx context.Context) *PoissonServer {
	server := &PoissonServer{
		System: runtime.NewSystem(ctx, "probability.poisson"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *PoissonServer) Write(ctx context.Context, call Poisson_write) error {
	args := call.Args()
	dist := distuv.Poisson{Lambda: args.Lambda()}
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
func (server *PoissonServer) Done(ctx context.Context, call Poisson_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.poisson.Done] failed to allocate done results",
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
