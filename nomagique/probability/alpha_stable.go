package probability

import (
	"context"
	"gonum.org/v1/gonum/stat/distuv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
AlphaStableServer calculates properties and samples from an alpha-stable continuous distribution.
*/
type AlphaStableServer struct {
	*runtime.System
	mean       float64
	variance   float64
	stdDev     float64
	median     float64
	mode       float64
	exKurtosis float64
	skewness   float64
	rand       float64
}

func NewAlphaStable(ctx context.Context) *AlphaStableServer {
	server := &AlphaStableServer{
		System: runtime.NewSystem(ctx, "probability.alpha_stable"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and distribution parameters.
*/
func (server *AlphaStableServer) Write(ctx context.Context, call AlphaStable_write) error {
	args := call.Args()
	dist := distuv.AlphaStable{
		Alpha: args.Alpha(),
		Beta:  args.Beta(),
		C:     args.C(),
		Mu:    args.Mu(),
	}
	server.mean = dist.Mean()
	server.variance = dist.Variance()
	server.stdDev = dist.StdDev()
	server.median = dist.Median()
	server.mode = dist.Mode()
	server.exKurtosis = dist.ExKurtosis()
	server.skewness = dist.Skewness()
	server.rand = dist.Rand()
	return nil
}

/*
Done returns calculated distribution results.
*/
func (server *AlphaStableServer) Done(ctx context.Context, call AlphaStable_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[probability.alpha_stable.Done] failed to allocate done results",
			err,
		))
	}

	results.SetMean(server.mean)
	results.SetVariance(server.variance)
	results.SetStdDev(server.stdDev)
	results.SetMedian(server.median)
	results.SetMode(server.mode)
	results.SetExKurtosis(server.exKurtosis)
	results.SetSkewness(server.skewness)
	results.SetRand(server.rand)
	return nil
}
