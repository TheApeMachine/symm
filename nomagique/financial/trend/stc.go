package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
StcServer calculates the Schaff Trend Cycle (STC).
*/
type StcServer struct {
	*runtime.System
	calculator *trend.Stc[float64]
	value chan float64
	out <-chan float64
	result float64
	count int
}

func NewStc(ctx context.Context) *StcServer {
	value := make(chan float64, 1)
	calculator := trend.NewStc[float64]()

	server := &StcServer{
		System: runtime.NewSystem(ctx, "financial.trend.stc"),
		calculator: calculator,
		value: value,
		out: calculator.ComputeWithContext(ctx, value),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *StcServer) Write(ctx context.Context, call Stc_write) error {
	valueVal := call.Args().Value()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case server.value <- valueVal:
	}

	server.count++

	if server.count > server.calculator.IdlePeriod() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-server.out:
			if !ok {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[financial.trend.stc.Write] calculator channel closed",
					nil,
				))
			}

			server.result = res
		}
	}

	return nil
}

/*
Done returns calculated indicator results.
*/
func (server *StcServer) Done(ctx context.Context, call Stc_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.stc.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
