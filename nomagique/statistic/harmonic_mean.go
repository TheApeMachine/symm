package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
HarmonicMeanServer calculates online harmonic mean of streaming nonzero observations.
*/
type HarmonicMeanServer struct {
	*runtime.System
	count    float64
	sumRecip float64
	result   float64
}

func NewHarmonicMean(ctx context.Context) *HarmonicMeanServer {
	server := &HarmonicMeanServer{
		System: runtime.NewSystem(ctx, "statistic.harmonic_mean"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *HarmonicMeanServer) Write(ctx context.Context, call HarmonicMean_write) error {
	valueVal := call.Args().Value()

	if valueVal != 0 {
		server.count++
		server.sumRecip += 1.0 / valueVal
		server.result = server.count / server.sumRecip
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *HarmonicMeanServer) Done(ctx context.Context, call HarmonicMean_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.harmonic_mean.Done] failed to allocate done results",
			err,
		))
	}

	results.SetHarmonicMean(server.result)
	return nil
}
