package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
KurtosisServer calculates online excess kurtosis of streaming observations.
*/
type KurtosisServer struct {
	*runtime.System
	count  float64
	mean   float64
	m2     float64
	m3     float64
	m4     float64
	result float64
}

func NewKurtosis(ctx context.Context) *KurtosisServer {
	server := &KurtosisServer{
		System: runtime.NewSystem(ctx, "statistic.kurtosis"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *KurtosisServer) Write(ctx context.Context, call Kurtosis_write) error {
	valueVal := call.Args().Value()
	server.count++

	if server.count == 1 {
		server.mean = valueVal
		return nil
	}

	delta := valueVal - server.mean
	deltaN := delta / server.count
	deltaN2 := deltaN * deltaN
	term1 := delta * deltaN * (server.count - 1)
	server.mean += deltaN
	server.m4 += term1*deltaN2*(server.count*server.count-3*server.count+3) + 6*deltaN2*server.m2 - 4*deltaN*server.m3
	server.m3 += term1*deltaN*(server.count-1) - 3*deltaN*server.m2
	server.m2 += term1

	if server.count > 3 && server.m2 > 0 {
		variance := server.m2 / (server.count - 1)
		server.result = (server.m4/server.count)/(variance*variance) - 3.0
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *KurtosisServer) Done(ctx context.Context, call Kurtosis_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.kurtosis.Done] failed to allocate done results",
			err,
		))
	}

	results.SetKurtosis(server.result)
	return nil
}
