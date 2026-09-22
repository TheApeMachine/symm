package trend

import (
	"context"

	"github.com/cinar/indicator/v2/trend"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
CfoServer calculates the Chande Forecast Oscillator (CFO).
*/
type CfoServer struct {
	*runtime.System
	calculator *trend.Cfo[float64]
	value      chan float64
	out        <-chan float64
	result     float64
	count      int
}

func NewCfo(ctx context.Context) *CfoServer {
	value := make(chan float64, 1)
	calculator := trend.NewCfo[float64]()

	server := &CfoServer{
		System:     runtime.NewSystem(ctx, "financial.trend.cfo"),
		calculator: calculator,
		value:      value,
		out:        calculator.ComputeWithContext(ctx, value),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observations.
*/
func (server *CfoServer) Write(ctx context.Context, call Cfo_write) error {
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
					"[financial.trend.cfo.Write] calculator channel closed",
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
func (server *CfoServer) Done(ctx context.Context, call Cfo_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[financial.trend.cfo.Done] failed to allocate done results",
			err,
		))
	}

	results.SetResult(server.result)
	return nil
}
