package statistic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ModeServer identifies the sample mode and frequency from streaming observations.
*/
type ModeServer struct {
	*runtime.System
	frequencies map[float64]int64
	mode float64
	maxFreq int64
}

func NewMode(ctx context.Context) *ModeServer {
	server := &ModeServer{
		System: runtime.NewSystem(ctx, "statistic.mode"),
		frequencies: make(map[float64]int64),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *ModeServer) Write(ctx context.Context, call Mode_write) error {
	valueVal := call.Args().Value()
	server.frequencies[valueVal]++

	if server.frequencies[valueVal] > server.maxFreq {
		server.maxFreq = server.frequencies[valueVal]
		server.mode = valueVal
	}
	return nil
}

/*
Done returns calculated results.
*/
func (server *ModeServer) Done(ctx context.Context, call Mode_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[statistic.mode.Done] failed to allocate done results",
			err,
		))
	}

	results.SetMode(server.mode)
	results.SetFrequency(server.maxFreq)
	return nil
}
