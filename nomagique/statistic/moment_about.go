package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MomentAboutServer calculates online r-th central moment about c of streaming observations.
*/
type MomentAboutServer struct {
	*runtime.System
	count  float64
	sumPow float64
	result float64
}

func NewMomentAbout(ctx context.Context) *MomentAboutServer {
	server := &MomentAboutServer{
		System: runtime.NewSystem(ctx, "statistic.moment_about"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *MomentAboutServer) Write(ctx context.Context, call MomentAbout_write) error {
	valueVal := call.Args().Value()
	orderVal := call.Args().Order()
	meanVal := call.Args().Mean()
	server.count++
	server.sumPow += math.Pow(valueVal-meanVal, orderVal)
	server.result = server.sumPow / server.count
	return nil
}

/*
Done returns calculated results.
*/
func (server *MomentAboutServer) Done(ctx context.Context, call MomentAbout_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.moment_about.Done] failed to allocate done results",
			err,
		))
	}

	results.SetMoment(server.result)
	return nil
}
