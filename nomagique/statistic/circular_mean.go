package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CircularMeanServer calculates online circular mean of streaming angles.
*/
type CircularMeanServer struct {
	*runtime.System
	count  float64
	sumSin float64
	sumCos float64
	result float64
}

func NewCircularMean(ctx context.Context) *CircularMeanServer {
	server := &CircularMeanServer{
		System: runtime.NewSystem(ctx, "statistic.circular_mean"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CircularMeanServer) Write(ctx context.Context, call CircularMean_write) error {
	angleVal := call.Args().Angle()
	server.count++
	server.sumSin += math.Sin(angleVal)
	server.sumCos += math.Cos(angleVal)
	server.result = math.Atan2(server.sumSin, server.sumCos)
	return nil
}

/*
Done returns calculated results.
*/
func (server *CircularMeanServer) Done(ctx context.Context, call CircularMean_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.circular_mean.Done] failed to allocate done results",
			err,
		))
	}

	results.SetMeanAngle(server.result)
	return nil
}
