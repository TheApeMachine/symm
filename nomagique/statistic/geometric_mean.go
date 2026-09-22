package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
GeometricMeanServer calculates online geometric mean of streaming positive observations.
*/
type GeometricMeanServer struct {
	*runtime.System
	count float64
	sumLog float64
	result float64
}

func NewGeometricMean(ctx context.Context) *GeometricMeanServer {
	server := &GeometricMeanServer{
		System: runtime.NewSystem(ctx, "statistic.geometric_mean"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *GeometricMeanServer) Write(ctx context.Context, call GeometricMean_write) error {
	valueVal := call.Args().Value()

	if valueVal > 0 {
		server.count++
		server.sumLog += math.Log(valueVal)
		server.result = math.Exp(server.sumLog / server.count)
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *GeometricMeanServer) Done(ctx context.Context, call GeometricMean_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.geometric_mean.Done] failed to allocate done results",
			err,
		))
	}

	results.SetGeometricMean(server.result)
	return nil
}
