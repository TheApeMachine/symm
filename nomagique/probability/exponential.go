package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ExponentialServer evaluates continuous Exponential memoryless distribution.
*/
type ExponentialServer struct {
	*runtime.System
	prob     float64
	logProb  float64
	cdf      float64
	quantile float64
	survival float64
	mean     float64
	variance float64
	stdDev   float64
	entropy  float64
	rand     float64
}

func NewExponential(ctx context.Context) *ExponentialServer {
	server := &ExponentialServer{
		System: runtime.NewSystem(ctx, "probability.exponential"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *ExponentialServer) Write(ctx context.Context, call Exponential_write) error {
	args := call.Args()
	dist := distuv.Exponential{Rate: args.Rate()}
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
	server.entropy = dist.Entropy()
	server.rand = dist.Rand()
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *ExponentialServer) Done(ctx context.Context, call Exponential_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.exponential.Done] failed to allocate done results",
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
	results.SetEntropy(server.entropy)
	results.SetRand(server.rand)
	return nil
}
