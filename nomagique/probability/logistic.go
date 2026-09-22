package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
LogisticServer evaluates continuous Logistic distribution.
*/
type LogisticServer struct {
	*runtime.System
	prob     float64
	logProb  float64
	cdf      float64
	quantile float64
	survival float64
	mean     float64
	variance float64
	stdDev   float64
}

func NewLogistic(ctx context.Context) *LogisticServer {
	server := &LogisticServer{
		System: runtime.NewSystem(ctx, "probability.logistic"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *LogisticServer) Write(ctx context.Context, call Logistic_write) error {
	args := call.Args()
	dist := distuv.Logistic{Mu: args.Mu(), S: args.S()}
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
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *LogisticServer) Done(ctx context.Context, call Logistic_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.logistic.Done] failed to allocate done results",
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
	return nil
}
