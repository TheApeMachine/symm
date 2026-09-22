package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
NormalServer evaluates continuous Normal Gaussian distribution.
*/
type NormalServer struct {
	*runtime.System
	prob float64
	logProb float64
	cdf float64
	quantile float64
	survival float64
	mean float64
	variance float64
	stdDev float64
	entropy float64
	rand float64
}

func NewNormal(ctx context.Context) *NormalServer {
	server := &NormalServer{
		System: runtime.NewSystem(ctx, "probability.normal"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *NormalServer) Write(ctx context.Context, call Normal_write) error {
	args := call.Args()
	dist := distuv.Normal{Mu: args.Mu(), Sigma: args.Sigma()}
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
func (server *NormalServer) Done(ctx context.Context, call Normal_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.normal.Done] failed to allocate done results",
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
