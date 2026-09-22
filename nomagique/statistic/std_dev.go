package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StdDevServer calculates online sample standard deviation of streaming observations.
*/
type StdDevServer struct {
	*runtime.System
	count float64
	mean float64
	m2 float64
	result float64
}

func NewStdDev(ctx context.Context) *StdDevServer {
	server := &StdDevServer{
		System: runtime.NewSystem(ctx, "statistic.std_dev"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *StdDevServer) Write(ctx context.Context, call StdDev_write) error {
	valueVal := call.Args().Value()
	server.count++
	delta := valueVal - server.mean
	server.mean += delta / server.count
	delta2 := valueVal - server.mean
	server.m2 += delta * delta2

	if server.count > 1 {
		server.result = math.Sqrt(server.m2 / (server.count - 1))
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *StdDevServer) Done(ctx context.Context, call StdDev_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.std_dev.Done] failed to allocate done results",
			err,
		))
	}

	results.SetStdDev(server.result)
	return nil
}
