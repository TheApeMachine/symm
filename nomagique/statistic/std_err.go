package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StdErrServer calculates online standard error of the mean of streaming observations.
*/
type StdErrServer struct {
	*runtime.System
	count  float64
	mean   float64
	m2     float64
	result float64
}

func NewStdErr(ctx context.Context) *StdErrServer {
	server := &StdErrServer{
		System: runtime.NewSystem(ctx, "statistic.std_err"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *StdErrServer) Write(ctx context.Context, call StdErr_write) error {
	valueVal := call.Args().Value()
	server.count++
	delta := valueVal - server.mean
	server.mean += delta / server.count
	delta2 := valueVal - server.mean
	server.m2 += delta * delta2

	if server.count > 1 {
		server.result = math.Sqrt(server.m2 / ((server.count - 1) * server.count))
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *StdErrServer) Done(ctx context.Context, call StdErr_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.std_err.Done] failed to allocate done results",
			err,
		))
	}

	results.SetStdErr(server.result)
	return nil
}
