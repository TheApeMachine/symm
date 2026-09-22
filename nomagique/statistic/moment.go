package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MomentServer calculates online r-th raw moment of streaming observations.
*/
type MomentServer struct {
	*runtime.System
	count  float64
	sumPow float64
	result float64
}

func NewMoment(ctx context.Context) *MomentServer {
	server := &MomentServer{
		System: runtime.NewSystem(ctx, "statistic.moment"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MomentServer) Write(ctx context.Context, call Moment_write) error {
	valueVal := call.Args().Value()
	orderVal := call.Args().Order()
	server.count++
	server.sumPow += math.Pow(valueVal, orderVal)
	server.result = server.sumPow / server.count
	return nil
}

/*
Done returns calculated results.
*/
func (server *MomentServer) Done(ctx context.Context, call Moment_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.moment.Done] failed to allocate done results",
			err,
		))
	}

	results.SetMoment(server.result)
	return nil
}
